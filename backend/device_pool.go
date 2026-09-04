package backend

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// PhysicalDevice is observed from the host. It is not yet an LWS device: the
// administrator must give it an LWS name before an app may receive it.
type PhysicalDevice struct {
	StableID    string            `json:"stableId"`
	CurrentPath string            `json:"currentPath"`
	Name        string            `json:"name"`
	Category    string            `json:"category"`
	Metadata    map[string]string `json:"metadata"`
}
type DeviceScanner interface {
	Scan(context.Context, bool) ([]PhysicalDevice, error)
}

type UdevDeviceScanner struct{ Runner CommandRunner }

func (s UdevDeviceScanner) Scan(ctx context.Context, includeSystem bool) ([]PhysicalDevice, error) {
	paths := []string{}
	if includeSystem {
		err := filepath.WalkDir("/dev", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if entry.IsDir() && (path == "/dev/fd" || path == "/dev/pts" || path == "/dev/shm" || path == "/dev/mqueue") {
				return filepath.SkipDir
			}
			info, err := entry.Info()
			if err == nil && info.Mode()&os.ModeDevice != 0 {
				paths = append(paths, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		for _, pattern := range []string{"/dev/hidraw*", "/dev/ttyUSB*", "/dev/ttyACM*"} {
			matches, err := filepath.Glob(pattern)
			if err != nil {
				return nil, err
			}
			paths = append(paths, matches...)
		}
	}
	result := []PhysicalDevice{}
	seenPaths := map[string]struct{}{}
	for _, path := range paths {
		if _, seen := seenPaths[path]; seen {
			continue
		}
		seenPaths[path] = struct{}{}
		out, err := s.Runner.Run(ctx, "udevadm", "info", "--query=property", "--name="+path)
		if err != nil {
			continue
		}
		properties := map[string]string{}
		for _, line := range strings.Split(string(out), "\n") {
			key, value, ok := strings.Cut(line, "=")
			if ok {
				properties[key] = value
			}
		}
		category := "system"
		if properties["ID_BUS"] == "usb" || strings.Contains(properties["DEVPATH"], "/usb") {
			category = "user"
		}
		if !includeSystem && category != "user" {
			continue
		}
		stable := properties["ID_SERIAL_SHORT"]
		if stable == "" {
			stable = properties["ID_SERIAL"]
		}
		if stable == "noserial" || stable == "no_serial" || stable == "unknown" || stable == "UNKNOWN" {
			stable = ""
		}
		if stable != "" {
			stable = "usb:" + stable
		} else if category == "system" {
			stable = "system:" + properties["DEVPATH"]
			if stable == "system:" {
				stable = "system:" + path
			}
		} else {
			// USB機器にserialがない場合でも候補から隠さない。USB topologyは
			// port変更で変わるため、UIではその旨を示し利用者に再確認させる。
			stable = "usb-path:" + properties["DEVPATH"]
		}
		name := strings.TrimSpace(strings.Join([]string{properties["ID_VENDOR_FROM_DATABASE"], properties["ID_MODEL_FROM_DATABASE"]}, " "))
		if name == "" {
			name = strings.TrimSpace(strings.Join([]string{properties["ID_VENDOR"], properties["ID_MODEL"]}, " "))
		}
		if name == "" {
			name = udevParentName(ctx, s.Runner, path)
		}
		if name == "" {
			name = filepath.Base(path)
		}
		identity := "serial"
		if strings.HasPrefix(stable, "usb-path:") {
			identity = "usb topology"
		}
		result = append(result, PhysicalDevice{StableID: stable, CurrentPath: path, Name: name, Category: category, Metadata: map[string]string{"vendor": properties["ID_VENDOR"], "model": properties["ID_MODEL"], "serial": properties["ID_SERIAL_SHORT"], "subsystem": properties["SUBSYSTEM"], "identity": identity, "type": fmt.Sprintf("%s device", category)}})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].StableID < result[j].StableID })
	return result, nil
}

func udevParentName(ctx context.Context, runner CommandRunner, path string) string {
	out, err := runner.Run(ctx, "udevadm", "info", "--attribute-walk", "--name="+path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		for _, key := range []string{"ATTRS{product}==\"", "ATTRS{manufacturer}==\""} {
			if strings.HasPrefix(line, key) {
				return strings.TrimSuffix(strings.TrimPrefix(line, key), "\"")
			}
		}
	}
	return ""
}

func RefreshLWSDevices(ctx context.Context, db *sql.DB, scanner DeviceScanner) ([]PhysicalDevice, error) {
	if scanner == nil {
		return nil, nil
	}
	candidates, err := scanner.Scan(ctx, true)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	for _, candidate := range candidates {
		paths[candidate.StableID] = candidate.CurrentPath
	}
	rows, err := db.QueryContext(ctx, `SELECT id,stable_id FROM lws_devices`)
	if err != nil {
		return candidates, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, stable string
		if rows.Scan(&id, &stable) != nil {
			continue
		}
		path, status := "", "DISCONNECTED"
		if current, ok := paths[stable]; ok {
			path, status = current, "CONNECTED"
		}
		if _, err := db.ExecContext(ctx, `UPDATE lws_devices SET current_path=?,status=?,updated_at=datetime('now') WHERE id=?`, path, status, id); err != nil {
			return candidates, err
		}
	}
	return candidates, rows.Err()
}
