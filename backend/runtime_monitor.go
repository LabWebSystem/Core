package backend

import (
	"context"
	"database/sql"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types/container"
	mobyclient "github.com/moby/moby/client"
)

const (
	ownerLabel          = "com.labwebsystem.owner"
	installationIDLabel = "com.labwebsystem.installation-id"
	applicationIDLabel  = "com.labwebsystem.app-id"
	composeServiceLabel = "com.docker.compose.service"
)

type MonitoredContainer struct {
	ID, Name string
	Labels   map[string]string
	TTY      bool
}
type DockerRuntimeEvent struct {
	ID, Action string
	Labels     map[string]string
	At         time.Time
}

// DockerRuntimeSourceはRuntimeMonitorをDocker SDKから分離し、収集境界をfakeで検証可能にする。
type DockerRuntimeSource interface {
	ListContainers(context.Context) ([]MonitoredContainer, error)
	Events(context.Context) (<-chan DockerRuntimeEvent, <-chan error)
	ContainerLogs(context.Context, string) (io.ReadCloser, error)
}

type MobyDockerRuntimeSource struct {
	client         *mobyclient.Client
	installationID string
}

func NewMobyDockerRuntimeSource(installationID string) (*MobyDockerRuntimeSource, error) {
	cli, err := mobyclient.New(mobyclient.FromEnv, mobyclient.WithAPIVersionNegotiation())
	if err != nil {
		return nil, err
	}
	return &MobyDockerRuntimeSource{client: cli, installationID: installationID}, nil
}

func (s *MobyDockerRuntimeSource) filters() mobyclient.Filters {
	filters := mobyclient.Filters{}
	filters.Add("label", ownerLabel+"=lws", installationIDLabel+"="+s.installationID)
	return filters
}
func (s *MobyDockerRuntimeSource) ListContainers(ctx context.Context) ([]MonitoredContainer, error) {
	result, err := s.client.ContainerList(ctx, mobyclient.ContainerListOptions{All: true, Filters: s.filters()})
	if err != nil {
		return nil, err
	}
	items := make([]MonitoredContainer, 0, len(result.Items))
	for _, item := range result.Items {
		inspect, err := s.client.ContainerInspect(ctx, item.ID, mobyclient.ContainerInspectOptions{})
		if err != nil {
			continue
		}
		items = append(items, monitoredContainer(item, inspect.Container.Config.Tty))
	}
	return items, nil
}
func monitoredContainer(item container.Summary, tty bool) MonitoredContainer {
	name := item.ID
	if len(item.Names) > 0 {
		name = strings.TrimPrefix(item.Names[0], "/")
	}
	return MonitoredContainer{ID: item.ID, Name: name, Labels: item.Labels, TTY: tty}
}
func (s *MobyDockerRuntimeSource) Events(ctx context.Context) (<-chan DockerRuntimeEvent, <-chan error) {
	result := s.client.Events(ctx, mobyclient.EventsListOptions{Filters: s.filters()})
	output := make(chan DockerRuntimeEvent)
	go func() {
		defer close(output)
		for message := range result.Messages {
			if string(message.Type) != "container" {
				continue
			}
			output <- DockerRuntimeEvent{ID: message.Actor.ID, Action: string(message.Action), Labels: message.Actor.Attributes, At: time.Unix(0, message.TimeNano).UTC()}
		}
	}()
	return output, result.Err
}
func (s *MobyDockerRuntimeSource) ContainerLogs(ctx context.Context, id string) (io.ReadCloser, error) {
	return s.client.ContainerLogs(ctx, id, mobyclient.ContainerLogsOptions{ShowStdout: true, ShowStderr: true, Follow: true, Tail: "100", Timestamps: true})
}

type RuntimeMonitor struct {
	Source         DockerRuntimeSource
	Logs           *LogStore
	DB             *sql.DB
	SecretKey      []byte
	InstallationID string
	BaseDomain     string
	RetryDelay     time.Duration
	mu             sync.Mutex
	tailing        map[string]context.CancelFunc
}

func NewRuntimeMonitor(source DockerRuntimeSource, logs *LogStore, db *sql.DB, installationID, baseDomain string, secretKey []byte) *RuntimeMonitor {
	return &RuntimeMonitor{Source: source, Logs: logs, DB: db, InstallationID: installationID, BaseDomain: baseDomain, SecretKey: secretKey, RetryDelay: time.Second, tailing: map[string]context.CancelFunc{}}
}
func (m *RuntimeMonitor) Run(ctx context.Context) {
	if m.Source == nil || m.Logs == nil {
		return
	}
	for {
		if ctx.Err() != nil {
			m.stopAll()
			return
		}
		containers, err := m.Source.ListContainers(ctx)
		if err == nil {
			for _, value := range containers {
				m.startContainer(ctx, value)
			}
		}
		m.consumeEvents(ctx)
		if ctx.Err() != nil {
			m.stopAll()
			return
		}
		delay := m.RetryDelay
		if delay <= 0 {
			delay = time.Second
		}
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-time.After(delay):
		}
	}
}
func (m *RuntimeMonitor) consumeEvents(ctx context.Context) {
	events, errors := m.Source.Events(ctx)
	for events != nil || errors != nil {
		select {
		case <-ctx.Done():
			return
		case value, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			if !m.owned(value.Labels) {
				continue
			}
			entry := m.entryFor(value.Labels, "", "Docker container "+value.Action)
			entry.OccurredAt = value.At
			if entry.OccurredAt.IsZero() {
				entry.OccurredAt = time.Now().UTC()
			}
			_, _ = m.Logs.Append(ctx, entry)
			if value.Action == "start" {
				m.startContainer(ctx, MonitoredContainer{ID: value.ID, Name: value.ID, Labels: value.Labels})
			}
			if value.Action == "die" || value.Action == "stop" || value.Action == "destroy" {
				m.stopContainer(value.ID)
			}
		case _, ok := <-errors:
			if !ok {
				errors = nil
				continue
			}
			return
		}
	}
}
func (m *RuntimeMonitor) startContainer(parent context.Context, value MonitoredContainer) {
	if value.ID == "" || !m.owned(value.Labels) {
		return
	}
	m.mu.Lock()
	if _, exists := m.tailing[value.ID]; exists {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.tailing[value.ID] = cancel
	m.mu.Unlock()
	go func() {
		defer m.stopContainer(value.ID)
		stream, err := m.Source.ContainerLogs(ctx, value.ID)
		if err != nil {
			return
		}
		defer stream.Close()
		secrets := m.applicationSecrets(ctx, value.Labels[applicationIDLabel])
		stdout := &runtimeLogWriter{monitor: m, context: ctx, entry: m.entryFor(value.Labels, value.Name, ""), level: "info", redactor: NewChunkRedactor(secrets)}
		stderr := &runtimeLogWriter{monitor: m, context: ctx, entry: m.entryFor(value.Labels, value.Name, ""), level: "info", redactor: NewChunkRedactor(secrets)}
		if value.TTY {
			_, _ = io.Copy(stdout, stream)
		} else {
			_, _ = stdcopy.StdCopy(stdout, stderr, stream)
		}
		stdout.Flush()
		stderr.Flush()
	}()
}
func (m *RuntimeMonitor) stopContainer(id string) {
	m.mu.Lock()
	if cancel, ok := m.tailing[id]; ok {
		delete(m.tailing, id)
		cancel()
	}
	m.mu.Unlock()
}
func (m *RuntimeMonitor) stopAll() {
	m.mu.Lock()
	values := m.tailing
	m.tailing = map[string]context.CancelFunc{}
	m.mu.Unlock()
	for _, cancel := range values {
		cancel()
	}
}
func (m *RuntimeMonitor) owned(labels map[string]string) bool {
	return labels[ownerLabel] == "lws" && labels[installationIDLabel] == m.InstallationID
}
func (m *RuntimeMonitor) entryFor(labels map[string]string, containerName, message string) StoredLogEntry {
	component := "backend"
	app := labels[applicationIDLabel]
	service := ""
	if app != "" {
		component = "application"
		service = labels[composeServiceLabel]
	} else {
		switch labels[composeServiceLabel] {
		case "caddy":
			component = "caddy"
		case "coredns":
			component = "coredns"
		case "dashboard":
			component = "dashboard"
		}
	}
	return StoredLogEntry{Component: component, Level: "info", ApplicationID: app, Service: service, ContainerName: containerName, Message: message}
}
func (m *RuntimeMonitor) applicationSecrets(ctx context.Context, app string) []string {
	if app == "" || m.DB == nil || len(m.SecretKey) == 0 {
		return nil
	}
	rows, err := m.DB.QueryContext(ctx, `SELECT value FROM application_variables WHERE application_id=? AND is_secret=1`, app)
	if err != nil {
		return nil
	}
	defer rows.Close()
	values := []string{}
	for rows.Next() {
		var encrypted []byte
		if rows.Scan(&encrypted) == nil {
			if plain, err := Decrypt(m.SecretKey, encrypted); err == nil {
				values = append(values, string(plain))
			}
		}
	}
	return values
}

type runtimeLogWriter struct {
	monitor  *RuntimeMonitor
	context  context.Context
	entry    StoredLogEntry
	level    string
	redactor *ChunkRedactor
	pending  string
}

func (w *runtimeLogWriter) Write(data []byte) (int, error) {
	safe := w.redactor.Feed(string(data))
	w.consume(safe)
	return len(data), nil
}
func (w *runtimeLogWriter) Flush() {
	w.consume(w.redactor.Flush())
	if w.pending != "" {
		w.write(strings.TrimSuffix(w.pending, "\n"))
		w.pending = ""
	}
}
func (w *runtimeLogWriter) consume(value string) {
	w.pending += value
	for {
		i := strings.IndexByte(w.pending, '\n')
		if i < 0 {
			return
		}
		w.write(w.pending[:i])
		w.pending = w.pending[i+1:]
	}
}
func (w *runtimeLogWriter) write(message string) {
	if message == "" {
		return
	}
	entry := w.entry
	entry.Level = classifyLogLevel(message, w.level)
	entry.Message = message
	w.monitor.associateHost(w.context, &entry)
	_, _ = w.monitor.Logs.Append(w.context, entry)
}

func (m *RuntimeMonitor) associateHost(ctx context.Context, entry *StoredLogEntry) {
	if entry.ApplicationID != "" || m.DB == nil || m.BaseDomain == "" || (entry.Component != "caddy" && entry.Component != "coredns" && entry.Component != "backend") {
		return
	}
	rows, err := m.DB.QueryContext(ctx, `SELECT id,subdomain FROM applications`)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var id, subdomain string
		if rows.Scan(&id, &subdomain) == nil && strings.Contains(entry.Message, subdomain+"."+m.BaseDomain) {
			entry.ApplicationID = id
			return
		}
	}
}
