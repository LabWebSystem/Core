import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, AlertTriangle, CirclePlus, ExternalLink, FileCog, LoaderCircle, Play, RotateCw, ScrollText, Settings2, Square, Trash2 } from "lucide-react";
import { api, ApiError, type Application, type LogEntry, type Operation } from "./api/client";

type PendingAction = { label: string; execute: () => Promise<{ name: string }> };
type LogView = "task" | "application" | "related";
type DisplayLog = { entry: LogEntry; message: string; lineCount: number; json?: string };

const appId = (app: Application) => app.name.replace("applications/", "");
const stateLabel = (app: Application) => app.reconciling ? "処理中" : app.observedState === "RUNNING" ? "稼働中" : app.observedState === "ERROR" ? "異常" : app.observedState === "UNKNOWN" ? "状態未確認" : "停止中";
const stateTone = (app: Application) => app.reconciling ? "busy" : app.observedState === "RUNNING" ? "running" : app.observedState === "ERROR" ? "error" : "stopped";
const observedStateLabel = (state: string) => ({ RUNNING: "稼働中", STOPPED: "停止中", ERROR: "異常", UNKNOWN: "状態未確認" })[state] ?? state;
const registrationStateLabel = (state: string) => ({ ACTIVE: "登録済み", UNREGISTERED: "登録解除済み" })[state] ?? state;
const publicUrl = (app: Application) => {
  const host = window.location.host.replace(/^dashboard\./, "");
  return `${window.location.protocol}//${app.subdomain}.${host}`;
};
const operationLabel = (kind?: string) => ({ create: "登録", update: "更新", configure: "設定更新", start: "開始", stop: "停止", sync: "同期", rebuild: "再構成", unregister: "登録解除", register: "再登録", purge: "完全削除" })[kind ?? ""] ?? "操作";

export function App() {
  const client = useQueryClient();
  const apps = useQuery({ queryKey: ["applications"], queryFn: api.list, refetchInterval: 5_000 });
  const [selectedId, setSelectedId] = useState<string>();
  const [registering, setRegistering] = useState(false);
  const [operation, setOperation] = useState<Operation>();
  const [logView, setLogView] = useState<LogView>("task");
  const [logService, setLogService] = useState<string>();
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [services, setServices] = useState<string[]>([]);
  const [message, setMessage] = useState<string>();
  const [pending, setPending] = useState<PendingAction>();
  const confirmRef = useRef<HTMLDialogElement>(null);
  const selected = registering ? undefined : apps.data?.find((app) => appId(app) === selectedId) ?? apps.data?.[0];

  useEffect(() => { if (pending) confirmRef.current?.showModal(); }, [pending]);
  useEffect(() => {
    if (!operation?.name || ["succeeded", "failed", "cancelled"].includes(operation.state)) return;
    return api.watchOperation(operation.name, (next) => {
      setOperation((current) => current ? { ...current, ...next } : { ...next, kind: "", errorMessage: "", createdAt: "", updatedAt: "" });
      if (["succeeded", "failed", "cancelled"].includes(next.state)) {
        void api.operation(next.name).then(setOperation).catch(() => undefined);
        void client.invalidateQueries({ queryKey: ["applications"] });
      }
    }, () => setMessage("操作の進行状況を再接続できません。しばらくしてから更新してください。"));
  }, [operation?.name, operation?.state, client]);
  useEffect(() => {
    setOperation(undefined);
    if (!selected?.latestOperation) return;
    let active = true;
    void api.operation(selected.latestOperation).then((next) => { if (active) setOperation(next); }).catch(() => undefined);
    return () => { active = false; };
  }, [selected?.latestOperation]);
  useEffect(() => {
    if (!selected) return;
    setLogs([]);
    return api.tailLogs(selected, logView, logView === "application" ? logService : undefined, (entry) => {
      if (entry.service) setServices((current) => current.includes(entry.service!) ? current : [...current, entry.service!].sort());
      setLogs((current) => [...current.slice(-199), entry]);
    });
  }, [selected?.name, logView, logService]);
  useEffect(() => { setServices([]); setLogService(undefined); }, [selected?.name]);

  const run = async (task: () => Promise<{ name: string }>) => {
    try {
      setMessage(undefined);
      const result = await task();
      setOperation(await api.operation(result.name));
      setLogView("task");
    } catch (error) {
      if (error instanceof ApiError && error.status === 409 && selected) {
        try {
          const current = await api.get(appId(selected));
          if (current.reconciling && current.latestOperation) {
            const activeOperation = await api.operation(current.latestOperation);
            setOperation(activeOperation);
            setMessage(`「${operationLabel(activeOperation.kind)}」が${activeOperation.state === "queued" ? "開始待ち" : "実行中"}です。完了するまでこのアプリの操作はできません。`);
            return;
          }
        } catch { /* 競合内容を取得できない場合はAPIのメッセージを表示する。 */ }
      }
      setMessage(error instanceof ApiError ? error.message : "通信に失敗しました");
    }
  };
  const register = useMutation({ mutationFn: api.create, onSuccess: (result) => { setRegistering(false); void run(async () => result); }, onError: (error) => setMessage(error instanceof ApiError ? error.message : "アプリを登録できませんでした。通信状態を確認して再試行してください") });
  const action = (kind: "start" | "stop" | "sync" | "rebuild" | "register") => selected && void run(() => api.action(selected, kind));
  const config = useQuery({ queryKey: ["configuration", selected?.name], queryFn: () => api.configuration(appId(selected!)), enabled: Boolean(selected) });
  const variableRows = useMemo(() => config.data?.variables ?? [], [config.data]);

  return <main className="app-shell">
    <header className="topbar"><div><span className="eyebrow">LAB WEB SYSTEM</span><h1>運用台帳</h1></div><div className="health"><Activity size={16} /> Backend {apps.isError ? "要確認" : "接続中"}</div></header>
    {message && <div className="notice" role="alert"><AlertTriangle size={17} />{message}<button onClick={() => setMessage(undefined)}>閉じる</button></div>}
    <section className="workbench">
      <aside className="ledger-pane"><div className="pane-title"><span>アプリ</span><b>{apps.data?.length ?? 0}</b></div><button className="new-app" onClick={() => { setRegistering(true); setSelectedId(undefined); setOperation(undefined); }}><CirclePlus size={17} />登録する</button><div className="app-list">{apps.isLoading && <p>台帳を読み込んでいます</p>}{apps.data?.map((app) => <button key={app.name} className={selected?.name === app.name ? "app-row active" : "app-row"} onClick={() => { setRegistering(false); setSelectedId(appId(app)); setOperation(undefined); }}><span className={`dot ${stateTone(app)}`} /><span><strong>{app.subdomain}</strong><small>{stateLabel(app)}</small></span></button>)}</div></aside>
      <section className="main-pane">{selected ? <AppWorkbench app={selected} variables={variableRows} configLoading={config.isLoading} onAction={action} onRun={run} onConfirm={(label, execute) => setPending({ label, execute })} /> : <RegisterForm busy={register.isPending} onSubmit={(input) => register.mutate(input)} />}</section>
      <aside className="activity-pane"><div className="pane-title"><span>操作とログ</span><ScrollText size={17} /></div><OperationView operation={operation} /><LogPane view={logView} service={logService} services={services} entries={logs} onViewChange={(view) => { setLogView(view); setLogService(undefined); }} onServiceChange={setLogService} /></aside>
    </section>
    <dialog ref={confirmRef} className="confirm-dialog"><form method="dialog"><AlertTriangle size={26} /><h2>{pending?.label}</h2><p>この操作は取り消せません。対象と影響を確認してから実行してください。</p><div className="dialog-actions"><button>戻る</button><button className="danger" onClick={(event) => { event.preventDefault(); const task = pending; confirmRef.current?.close(); setPending(undefined); if (task) void run(task.execute); }}>実行する</button></div></form></dialog>
  </main>;
}

function RegisterForm({ busy, onSubmit }: { busy: boolean; onSubmit: (input: { repositoryUrl: string; ref: string; subdomain: string }) => void }) {
  const [repositoryUrl, setRepositoryUrl] = useState(""); const [ref, setRef] = useState("main"); const [subdomain, setSubdomain] = useState("test-app");
  return <section className="empty-workbench"><div className="empty-icon"><CirclePlus /></div><span className="eyebrow">最初のアプリ</span><h2>台帳にアプリを登録する</h2><p>GitHub リポジトリの Compose 定義を確認して、LAN 内の URL を準備します。</p><form className="register-form" onSubmit={(event) => { event.preventDefault(); onSubmit({ repositoryUrl, ref, subdomain }); }}><label>GitHub リポジトリ<input required type="url" placeholder="https://github.com/owner/repository" value={repositoryUrl} onChange={(e) => setRepositoryUrl(e.target.value)} /></label><div className="form-row"><label>ブランチまたはタグ<input required value={ref} onChange={(e) => setRef(e.target.value)} /></label><label>公開名<input required pattern="[a-z0-9][a-z0-9-]{0,61}" value={subdomain} onChange={(e) => setSubdomain(e.target.value)} /></label></div><button className="primary" disabled={busy}>{busy ? <LoaderCircle className="spin" size={18} /> : <CirclePlus size={18} />}検証して登録</button></form></section>;
}

function AppWorkbench({ app, variables, configLoading, onAction, onRun, onConfirm }: { app: Application; variables: { name: string; isSecret: boolean }[]; configLoading: boolean; onAction: (kind: "start" | "stop" | "sync" | "rebuild" | "register") => void; onRun: (task: () => Promise<{ name: string }>) => void; onConfirm: (label: string, execute: () => Promise<{ name: string }>) => void }) {
  const [values, setValues] = useState<Record<string, { value: string; secret: boolean }>>({});
  const [newName, setNewName] = useState(""); const [newValue, setNewValue] = useState(""); const [newSecret, setNewSecret] = useState(false);
  const rows = [...variables.map((item) => ({ ...item, value: values[item.name]?.value ?? "" })), ...(newName ? [{ name: newName, isSecret: newSecret, value: newValue }] : [])];
  const disabled = app.reconciling;
  return <section className="app-workbench"><header className="app-heading"><div><span className="eyebrow">公開アプリ</span><h2>{app.subdomain}</h2><a href={publicUrl(app)} target="_blank" rel="noreferrer">{publicUrl(app)} <ExternalLink size={14} /></a></div><span className={`state-pill ${stateTone(app)}`}>{stateLabel(app)}</span></header><div className="action-strip" aria-label="アプリ操作"><button disabled={disabled} title={disabled ? "実行中の操作が完了するまで待ってください" : undefined} onClick={() => onAction("start")}><Play size={16} />開始</button><button disabled={disabled} title={disabled ? "実行中の操作が完了するまで待ってください" : undefined} onClick={() => onAction("stop")}><Square size={16} />停止</button><button disabled={disabled} title={disabled ? "実行中の操作が完了するまで待ってください" : undefined} onClick={() => onAction("sync")}><RotateCw size={16} />同期</button><button disabled={disabled} title={disabled ? "実行中の操作が完了するまで待ってください" : undefined} onClick={() => onAction("rebuild")}><Settings2 size={16} />再構成</button><button disabled={disabled} title={disabled ? "実行中の操作が完了するまで待ってください" : undefined} onClick={() => onConfirm("登録を解除する", () => api.unregister(app))}><FileCog size={16} />登録解除</button><button className="danger-outline" disabled={disabled} title={disabled ? "実行中の操作が完了するまで待ってください" : undefined} onClick={() => onConfirm("アプリと保存データを完全に削除する", () => api.purge(app))}><Trash2 size={16} />完全削除</button></div><section className="details-grid"><div><span>リポジトリ</span><code>{app.repositoryUrl}</code></div><div><span>参照</span><code>{app.ref}</code></div><div><span>実行状態</span><strong>{observedStateLabel(app.observedState)}</strong></div><div><span>登録状態</span><strong>{registrationStateLabel(app.registrationState)}</strong></div>{app.latestError && <div><span>異常内容</span><strong>{app.latestError}</strong></div>}<div><span>最終確認</span><time>{new Date(app.observedAt).toLocaleString("ja-JP")}</time></div></section><section className="settings-section"><div className="section-heading"><div><span className="eyebrow">環境設定</span><h3>変数</h3></div><button className="primary small" disabled={disabled} onClick={() => onConfirm("環境設定を更新する", () => api.saveConfiguration(app, Object.fromEntries(rows.filter((row) => row.value).map((row) => [row.name, { value: row.value, secret: row.isSecret }]))))}>保存</button></div>{configLoading ? <p>設定を読み込んでいます</p> : <><div className="variable-list">{rows.map((row) => <label key={row.name} className="variable-row"><code>{row.name}</code><input disabled={disabled} type={row.isSecret ? "password" : "text"} value={row.value} placeholder={row.isSecret ? "secret は表示されません" : "値を入力"} onChange={(event) => setValues((current) => ({ ...current, [row.name]: { value: event.target.value, secret: row.isSecret } }))} /><small>{row.isSecret ? "secret" : "通常値"}</small></label>)}</div><div className="add-variable"><input disabled={disabled} placeholder="変数名" value={newName} onChange={(e) => setNewName(e.target.value.toUpperCase())} /><input disabled={disabled} placeholder="値" value={newValue} onChange={(e) => setNewValue(e.target.value)} /><label><input disabled={disabled} type="checkbox" checked={newSecret} onChange={(e) => setNewSecret(e.target.checked)} />secret</label></div></>}</section></section>;
}

function OperationView({ operation }: { operation?: Operation }) { return <section className="operation" aria-live="polite"><span className="eyebrow">現在の操作</span>{operation ? <><strong>{operationLabel(operation.kind)}: {operation.state === "queued" ? "開始待ち" : operation.state === "running" ? "実行中" : operation.state === "succeeded" ? "完了" : operation.state === "cancelled" ? "中止" : "失敗"}</strong><small>{operation.name}</small><time>{operation.createdAt ? `開始: ${new Date(operation.createdAt).toLocaleString("ja-JP")}` : "開始時刻を確認中です"}</time><p>{operation.errorMessage || operation.displayMessage || (operation.state === "running" || operation.state === "queued" ? "進捗を確認しています。" : "")}</p></> : <p>実行中の操作はありません。</p>}</section>; }

function LogPane({ view, service, services, entries, onViewChange, onServiceChange }: { view: LogView; service?: string; services: string[]; entries: LogEntry[]; onViewChange: (view: LogView) => void; onServiceChange: (service?: string) => void }) {
  const labels: Record<LogView, string> = { task: "タスクログ", application: "アプリログ", related: "関連ログ" };
  const displayEntries = groupLogEntries(entries);
  return <section className="log-pane" aria-label="アプリ運用ログ"><div className="log-tabs" role="tablist" aria-label="ログ種別">{(Object.keys(labels) as LogView[]).map((item) => <button key={item} role="tab" aria-selected={view === item} onClick={() => onViewChange(item)}>{labels[item]}</button>)}</div>{view === "application" && <label className="service-select">対象コンテナ<select value={service ?? ""} onChange={(event) => onServiceChange(event.target.value || undefined)}><option value="">全てのコンテナ</option>{services.map((item) => <option key={item} value={item}>{item}</option>)}</select></label>}<ol className="log-list" aria-live="polite">{displayEntries.length ? displayEntries.map((item) => <LogRow key={item.entry.id} item={item} view={view} />) : <li className="log-empty">{labels[view]}はまだありません。</li>}</ol></section>;
}

export function groupLogEntries(entries: LogEntry[]): DisplayLog[] {
  const groups: DisplayLog[] = [];
  for (let index = 0; index < entries.length; index += 1) {
    const entry = entries[index];
    const task = logTask(entry.message);
    const lines = [logBody(entry.message)];
    let json: string | undefined;
    if (/^[{[]/.test(lines[0].trim())) {
      for (let next = index; next < entries.length && next < index + 100; next += 1) {
        if (next > index) {
          const candidate = entries[next];
          if (candidate.component !== entry.component || candidate.service !== entry.service || candidate.containerName !== entry.containerName || candidate.level !== entry.level || logTask(candidate.message) !== task) break;
          lines.push(logBody(candidate.message));
        }
        try {
          json = JSON.stringify(JSON.parse(lines.join("\n")), null, 2);
          index = next;
          break;
        } catch { /* 完結したJSONになるまで次の連続行を確認する。 */ }
      }
    }
    groups.push({ entry, message: lines.join("\n"), lineCount: lines.length, json });
  }
  return groups;
}

function logTask(message: string) { return message.match(/^\[([^\]]+)\]\s*/)?.[1] ?? ""; }
function logBody(message: string) { return message.replace(/^\[[^\]]+\]\s*/, ""); }

function LogRow({ item, view }: { item: DisplayLog; view: LogView }) {
  const { entry } = item;
  const subject = view === "task" ? logTask(entry.message) || "操作" : view === "application" ? entry.service ?? "container" : entry.component;
  return <li className={`log-row level-${entry.level}`}><div className="log-meta"><time dateTime={entry.occurredAt}>{new Date(entry.occurredAt).toLocaleTimeString("ja-JP")}</time><span className="log-level">{entry.level}</span><span className="log-subject">{subject}</span></div>{item.json ? <details className="log-json"><summary>構造化ログ <small>{item.lineCount}行</small></summary><pre>{item.json}</pre></details> : <p>{item.message.replace(/^\[[^\]]+\]\s*/, "")}</p>}</li>;
}
