package backend

import "net/http"

// generatedAPIはOpenAPI生成server interfaceと業務handlerを接続する薄いadapterです。
// routeと引数の定義はopenapi.yaml・生成物を正本とし、業務処理はServerへ委譲します。
type generatedAPI struct{ server *Server }

var _ ServerInterface = generatedAPI{}

func (a generatedAPI) ListApplications(w http.ResponseWriter, r *http.Request) {
	a.server.listApplications(w, r)
}
func (a generatedAPI) CreateApplication(w http.ResponseWriter, r *http.Request) {
	a.server.createApplication(w, r)
}
func (a generatedAPI) UnregisterApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.deleteApplication(w, withApplicationPath(r, application))
}
func (a generatedAPI) GetApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.getApplication(w, withApplicationPath(r, application))
}
func (a generatedAPI) UpdateApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.patchApplication(w, withApplicationPath(r, application))
}
func (a generatedAPI) GetConfiguration(w http.ResponseWriter, r *http.Request, application string) {
	a.server.getConfiguration(w, withApplicationPath(r, application))
}
func (a generatedAPI) UpdateConfiguration(w http.ResponseWriter, r *http.Request, application string) {
	a.server.patchConfiguration(w, withApplicationPath(r, application))
}
func (a generatedAPI) PurgeApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.appOperation(w, withApplicationPath(r, application))
}
func (a generatedAPI) RebuildApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.appOperation(w, withApplicationPath(r, application))
}
func (a generatedAPI) StartApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.appOperation(w, withApplicationPath(r, application))
}
func (a generatedAPI) RegisterApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.appOperation(w, withApplicationPath(r, application))
}
func (a generatedAPI) StopApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.appOperation(w, withApplicationPath(r, application))
}
func (a generatedAPI) SyncApplication(w http.ResponseWriter, r *http.Request, application string) {
	a.server.appOperation(w, withApplicationPath(r, application))
}
func (a generatedAPI) ListLogEntries(w http.ResponseWriter, r *http.Request, application string, _ ListLogEntriesParams) {
	a.server.listLogEntries(w, withApplicationPath(r, application))
}
func (a generatedAPI) WatchLogEntries(w http.ResponseWriter, r *http.Request, application string, _ WatchLogEntriesParams) {
	a.server.watchLogEntries(w, withApplicationPath(r, application))
}
func (a generatedAPI) HealthLive(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (a generatedAPI) HealthReady(w http.ResponseWriter, _ *http.Request) {
	ready := a.server.DB != nil && a.server.DB.Ping() == nil
	if a.server.Ready != nil {
		ready = ready && a.server.Ready()
	}
	if ready {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
}
func (a generatedAPI) GetOperation(w http.ResponseWriter, r *http.Request, operation string) {
	a.server.getOperation(w, withOperationPath(r, operation))
}
func (a generatedAPI) WatchOperation(w http.ResponseWriter, r *http.Request, operation string) {
	a.server.watchOperation(w, withOperationPath(r, operation))
}

func withApplicationPath(r *http.Request, value string) *http.Request {
	r.SetPathValue("application", value)
	return r
}

func withOperationPath(r *http.Request, value string) *http.Request {
	r.SetPathValue("operation", value)
	return r
}
