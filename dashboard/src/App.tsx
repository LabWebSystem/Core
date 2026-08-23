import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, AlertTriangle, CirclePlus, ExternalLink, FileCog, LoaderCircle, Play, RotateCw, ScrollText, Settings2, Square, Trash2 } from "lucide-react";
import { api, ApiError, type Application, type Operation } from "./api/client";

type PendingAction = { label: string; execute: () => Promise<{ name: string }> };

const appId = (app: Application) => app.name.replace("applications/", "");
const stateLabel = (app: Application) => app.reconciling ? "処理中" : app.observedState === "RUNNING" ? "稼働中" : "停止中";
const publicUrl = (app: Application) => {
  const host = window.location.host.replace(/^dashboard\./, "");
  return `${window.location.protocol}//${app.subdomain}.${host}`;
};

export function App() {
  const client = useQueryClient();
  const apps = useQuery({ queryKey: ["applications"], queryFn: api.list, refetchInterval: 5_000 });
  const [selectedId, setSelectedId] = useState<string>();
  const [operation, setOperation] = useState<Operation>();
  const [logs, setLogs] = useState<string[]>([]);
  const [message, setMessage] = useState<string>();
  const [pending, setPending] = useState<PendingAction>();
  const confirmRef = useRef<HTMLDialogElement>(null);
  const selected = apps.data?.find((app) => appId(app) === selectedId) ?? apps.data?.[0];

  useEffect(() => { if (pending) confirmRef.current?.showModal(); }, [pending]);
  useEffect(() => {
    if (!operation?.name || ["succeeded", "failed", "cancelled"].includes(operation.state)) return;
    return api.watchOperation(operation.name, (next) => {
      setOperation(next);
      if (["succeeded", "failed", "cancelled"].includes(next.state)) void client.invalidateQueries({ queryKey: ["applications"] });
    }, () => setMessage("操作の進行状況を再接続できません。しばらくしてから更新してください。"));
  }, [operation?.name, operation?.state, client]);
  useEffect(() => {
    if (!selected) return;
    setLogs([]);
    return api.tailLogs(selected, (line) => setLogs((current) => [...current.slice(-199), line]));
  }, [selected?.name]);

  const run = async (task: () => Promise<{ name: string }>) => {
    try {
      setMessage(undefined);
      const result = await task();
      setOperation({ name: result.name, state: "queued" });
    } catch (error) { setMessage(error instanceof ApiError ? error.message : "通信に失敗しました"); }
  };
  const register = useMutation({ mutationFn: api.create, onSuccess: (result) => void run(async () => result), onError: () => setMessage("アプリを登録できませんでした") });
  const action = (kind: "start" | "stop" | "sync" | "rebuild" | "register") => selected && void run(() => api.action(selected, kind));
  const config = useQuery({ queryKey: ["configuration", selected?.name], queryFn: () => api.configuration(appId(selected!)), enabled: Boolean(selected) });
  const variableRows = useMemo(() => config.data?.variables ?? [], [config.data]);

  return <main className="app-shell">
    <header className="topbar"><div><span className="eyebrow">LAB WEB SYSTEM</span><h1>運用台帳</h1></div><div className="health"><Activity size={16} /> Backend {apps.isError ? "要確認" : "接続中"}</div></header>
    {message && <div className="notice" role="alert"><AlertTriangle size={17} />{message}<button onClick={() => setMessage(undefined)}>閉じる</button></div>}
    <section className="workbench">
      <aside className="ledger-pane"><div className="pane-title"><span>アプリ</span><b>{apps.data?.length ?? 0}</b></div><button className="new-app" onClick={() => setSelectedId(undefined)}><CirclePlus size={17} />登録する</button><div className="app-list">{apps.isLoading && <p>台帳を読み込んでいます</p>}{apps.data?.map((app) => <button key={app.name} className={selected?.name === app.name ? "app-row active" : "app-row"} onClick={() => setSelectedId(appId(app))}><span className={`dot ${app.reconciling ? "busy" : app.observedState === "RUNNING" ? "running" : "stopped"}`} /><span><strong>{app.subdomain}</strong><small>{stateLabel(app)}</small></span></button>)}</div></aside>
      <section className="main-pane">{selected ? <AppWorkbench app={selected} variables={variableRows} configLoading={config.isLoading} onAction={action} onRun={run} onConfirm={(label, execute) => setPending({ label, execute })} /> : <RegisterForm busy={register.isPending} onSubmit={(input) => register.mutate(input)} />}</section>
      <aside className="activity-pane"><div className="pane-title"><span>操作とログ</span><ScrollText size={17} /></div><OperationView operation={operation} /><pre className="log-stream" aria-label="コンテナログ">{logs.length ? logs.join("\n") : "選択したアプリのログがここに表示されます。"}</pre></aside>
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
  return <section className="app-workbench"><header className="app-heading"><div><span className="eyebrow">公開アプリ</span><h2>{app.subdomain}</h2><a href={publicUrl(app)} target="_blank" rel="noreferrer">{publicUrl(app)} <ExternalLink size={14} /></a></div><span className={`state-pill ${app.reconciling ? "busy" : app.observedState === "RUNNING" ? "running" : "stopped"}`}>{stateLabel(app)}</span></header><div className="action-strip"><button onClick={() => onAction("start")}><Play size={16} />開始</button><button onClick={() => onAction("stop")}><Square size={16} />停止</button><button onClick={() => onAction("sync")}><RotateCw size={16} />同期</button><button onClick={() => onAction("rebuild")}><Settings2 size={16} />再構成</button><button onClick={() => onConfirm("登録を解除する", () => api.unregister(app))}><FileCog size={16} />登録解除</button><button className="danger-outline" onClick={() => onConfirm("アプリと保存データを完全に削除する", () => api.purge(app))}><Trash2 size={16} />完全削除</button></div><section className="details-grid"><div><span>リポジトリ</span><code>{app.repositoryUrl}</code></div><div><span>参照</span><code>{app.ref}</code></div><div><span>希望状態</span><strong>{app.desiredState}</strong></div><div><span>最終確認</span><time>{new Date(app.observedAt).toLocaleString("ja-JP")}</time></div></section><section className="settings-section"><div className="section-heading"><div><span className="eyebrow">環境設定</span><h3>変数</h3></div><button className="primary small" onClick={() => onConfirm("環境設定を更新する", () => api.saveConfiguration(app, Object.fromEntries(rows.filter((row) => row.value).map((row) => [row.name, { value: row.value, secret: row.isSecret }]))))}>保存</button></div>{configLoading ? <p>設定を読み込んでいます</p> : <><div className="variable-list">{rows.map((row) => <label key={row.name} className="variable-row"><code>{row.name}</code><input type={row.isSecret ? "password" : "text"} value={row.value} placeholder={row.isSecret ? "secret は表示されません" : "値を入力"} onChange={(event) => setValues((current) => ({ ...current, [row.name]: { value: event.target.value, secret: row.isSecret } }))} /><small>{row.isSecret ? "secret" : "通常値"}</small></label>)}</div><div className="add-variable"><input placeholder="変数名" value={newName} onChange={(e) => setNewName(e.target.value.toUpperCase())} /><input placeholder="値" value={newValue} onChange={(e) => setNewValue(e.target.value)} /><label><input type="checkbox" checked={newSecret} onChange={(e) => setNewSecret(e.target.checked)} />secret</label></div></>}</section></section>;
}

function OperationView({ operation }: { operation?: Operation }) { return <section className="operation" aria-live="polite"><span className="eyebrow">現在の操作</span>{operation ? <><strong>{operation.state === "queued" ? "待機中" : operation.state === "running" ? "実行中" : operation.state === "succeeded" ? "完了" : operation.state === "cancelled" ? "中止" : "失敗"}</strong><small>{operation.name}</small><p>{operation.errorMessage || (operation.state === "running" ? "処理を実行しています。完了までこのままお待ちください。" : "")}</p></> : <p>まだ操作はありません。</p>}</section>; }
