import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CirclePlus,
  Database,
  ExternalLink,
  FileCog,
  LoaderCircle,
  Network,
  Play,
  RotateCw,
  ScrollText,
  Settings2,
  Square,
  Trash2,
  Usb,
  X,
} from "lucide-react";
import {
  api,
  ApiError,
  type Application,
  type Configuration,
  type CreateApplicationInput,
  type LogEntry,
  type LogQuery,
  type Operation,
  type ResourcePools,
} from "./api/client";

type PendingAction = {
  label: string;
  detail?: string;
  execute: () => Promise<{ name: string }>;
};
type LogView = "task" | "application" | "related";
type DisplayLog = {
  entry: LogEntry;
  message: string;
  lineCount: number;
  json?: string;
};

const appId = (app: Application) => app.name.replace("applications/", "");
const stateLabel = (app: Application) =>
  app.registrationState === "UNREGISTERED"
    ? "登録解除済み"
    : app.registrationState === "CONFIGURING"
      ? "設定待ち"
      : app.reconciling
        ? "処理中"
        : app.observedState === "RUNNING"
          ? "稼働中"
          : app.observedState === "ERROR"
            ? "異常"
            : app.observedState === "UNKNOWN"
              ? "状態未確認"
              : "停止中";
const stateTone = (app: Application) =>
  app.reconciling
    ? "busy"
    : app.observedState === "RUNNING"
      ? "running"
      : app.observedState === "ERROR"
        ? "error"
        : "stopped";
const observedStateLabel = (state: string) =>
  ({
    RUNNING: "稼働中",
    STOPPED: "停止中",
    ERROR: "異常",
    UNKNOWN: "状態未確認",
  })[state] ?? state;
const registrationStateLabel = (state: string) =>
  ({
    ACTIVE: "登録済み",
    CONFIGURING: "設定待ち",
    UNREGISTERED: "登録解除済み",
  })[state] ?? state;
const publicUrl = (app: Application) => {
  const host = window.location.host.replace(/^dashboard\./, "");
  return `${window.location.protocol}//${app.subdomain}.${host}`;
};
const operationLabel = (kind?: string) =>
  ({
    create: "登録",
    update: "更新",
    configure: "設定更新",
    start: "開始",
    stop: "停止",
    sync: "同期",
    rebuild: "再構成",
    unregister: "登録解除",
    register: "再登録",
    purge: "完全削除",
  })[kind ?? ""] ?? "操作";
const phaseLabel = (phase?: string) =>
  ({
    starting: "開始",
    source_prepare: "source準備",
    runtime_prepare: "実行設定",
    compose_up: "Compose起動",
    publish: "公開設定",
    unregister: "登録解除",
    volume_cleanup: "Docker volume削除",
    filesystem_cleanup: "作業領域削除",
    database_cleanup: "保存データ削除",
    queued: "開始待ち",
    running: "実行中",
    succeeded: "完了",
    failed: "失敗",
    cancelled: "中止",
  })[phase ?? ""] ?? phase;

export function App() {
  const client = useQueryClient();
  const apps = useQuery({
    queryKey: ["applications"],
    queryFn: api.list,
    refetchInterval: 5_000,
  });
  const [selectedId, setSelectedId] = useState<string>();
  const [registering, setRegistering] = useState(false);
  const [resourceView, setResourceView] = useState(false);
  const [operation, setOperation] = useState<Operation>();
  const [logView, setLogView] = useState<LogView>("task");
  const [logService, setLogService] = useState<string>();
  const [logs, setLogs] = useState<LogEntry[]>([]);
  const [logQuery, setLogQuery] = useState<LogQuery>({ limit: 200 });
  const [services, setServices] = useState<string[]>([]);
  const [message, setMessage] = useState<string>();
  const [pending, setPending] = useState<PendingAction>();
  const confirmRef = useRef<HTMLDialogElement>(null);
  const selected = registering
    ? undefined
    : (apps.data?.find((app) => appId(app) === selectedId) ?? apps.data?.[0]);

  useEffect(() => {
    if (pending) confirmRef.current?.showModal();
  }, [pending]);
  useEffect(() => {
    if (!operation?.name || ["succeeded", "failed", "cancelled"].includes(operation.state)) return;
    return api.watchOperation(
      operation.name,
      (next) => {
        setOperation((current) =>
          current
            ? { ...current, ...next }
            : {
                ...next,
                kind: "",
                errorMessage: "",
                createdAt: "",
                updatedAt: "",
              },
        );
        if (["succeeded", "failed", "cancelled"].includes(next.state)) {
          void api
            .operation(next.name)
            .then(setOperation)
            .catch(() => undefined);
          void client.invalidateQueries({ queryKey: ["applications"] });
        }
      },
      () => setMessage("操作の進行状況を再接続できません。しばらくしてから更新してください。"),
    );
  }, [operation?.name, operation?.state, client]);
  useEffect(() => {
    setOperation(undefined);
    if (!selected?.latestOperation) return;
    let active = true;
    void api
      .operation(selected.latestOperation)
      .then((next) => {
        if (active) setOperation(next);
      })
      .catch(() => undefined);
    return () => {
      active = false;
    };
  }, [selected?.latestOperation]);
  useEffect(() => {
    if (!selected) return;
    setLogs([]);
    return api.tailLogs(
      selected,
      logView,
      logView === "application" ? logService : undefined,
      logQuery,
      (entry) => {
        if (entry.service)
          setServices((current) =>
            current.includes(entry.service!) ? current : [...current, entry.service!].sort(),
          );
        setLogs((current) => [...current.slice(-(logQuery.limit ?? 200) + 1), entry]);
      },
    );
  }, [selected?.name, logView, logService, logQuery]);
  useEffect(() => {
    setServices([]);
    setLogService(undefined);
  }, [selected?.name]);

  const run = async (task: () => Promise<{ name: string }>) => {
    try {
      setMessage(undefined);
      const result = await task();
      const nextOperation = await api.operation(result.name);
      setOperation(nextOperation);
      if (["succeeded", "failed", "cancelled"].includes(nextOperation.state)) {
        void client.invalidateQueries({ queryKey: ["applications"] });
      }
      setLogView("task");
    } catch (error) {
      if (error instanceof ApiError && error.status === 409 && selected) {
        try {
          const current = await api.get(appId(selected));
          if (current.reconciling && current.latestOperation) {
            const activeOperation = await api.operation(current.latestOperation);
            setOperation(activeOperation);
            setMessage(
              `「${operationLabel(activeOperation.kind)}」が${activeOperation.state === "queued" ? "開始待ち" : "実行中"}です。完了するまでこのアプリの操作はできません。`,
            );
            return;
          }
        } catch {
          /* 競合内容を取得できない場合はAPIのメッセージを表示する。 */
        }
      }
      setMessage(error instanceof ApiError ? error.message : "通信に失敗しました");
    }
  };
  const register = useMutation({
    mutationFn: api.create,
    onSuccess: (result) => {
      setRegistering(false);
      void run(async () => result);
    },
    onError: (error) =>
      setMessage(
        error instanceof ApiError
          ? error.message
          : "アプリを登録できませんでした。通信状態を確認して再試行してください",
      ),
  });
  const action = (kind: "start" | "stop" | "sync" | "rebuild" | "register") =>
    selected && void run(() => api.action(selected, kind));
  const config = useQuery({
    queryKey: ["configuration", selected?.name],
    queryFn: () => api.configuration(appId(selected!)),
    enabled: Boolean(selected),
  });
  const configurationPools = useQuery({
    queryKey: ["resource-pools"],
    queryFn: () => api.resourcePools(),
    enabled: Boolean(selected),
  });
  const pools = useQuery({
    queryKey: ["resource-pools"],
    queryFn: () => api.resourcePools(),
    enabled: resourceView,
  });
  const configuration = useMemo(() => config.data, [config.data]);

  return (
    <main className="app-shell">
      <header className="topbar">
        <div>
          <span className="eyebrow">LAB WEB SYSTEM</span>
          <h1>運用台帳</h1>
          <p className="configuration-guide">
            登録後は設定レイヤーで環境変数とデバイスを確認してから開始します。
          </p>
        </div>
        <div className="health">
          <Activity size={16} /> Backend {apps.isError ? "要確認" : "接続中"}
        </div>
      </header>
      {message && (
        <div className="notice" role="alert">
          <AlertTriangle size={17} />
          {message}
          <button onClick={() => setMessage(undefined)}>閉じる</button>
        </div>
      )}
      <section className="workbench">
        <aside className="ledger-pane">
          <div className="pane-title">
            <span>アプリ</span>
            <b>{apps.data?.length ?? 0}</b>
          </div>
          <button
            className="new-app"
            onClick={() => {
              setResourceView(true);
              setRegistering(false);
            }}
          >
            <Database size={17} />
            リソースプール
          </button>
          <button
            className="new-app"
            onClick={() => {
              setResourceView(false);
              setRegistering(true);
              setSelectedId(undefined);
              setOperation(undefined);
            }}
          >
            <CirclePlus size={17} />
            登録する
          </button>
          <div className="app-list">
            {apps.isLoading && <p>台帳を読み込んでいます</p>}
            {apps.data?.map((app) => (
              <button
                key={app.name}
                className={
                  !resourceView && selected?.name === app.name ? "app-row active" : "app-row"
                }
                onClick={() => {
                  setResourceView(false);
                  setRegistering(false);
                  setSelectedId(appId(app));
                  setOperation(undefined);
                }}
              >
                <span className={`dot ${stateTone(app)}`} />
                <span>
                  <strong>{app.subdomain}</strong>
                  <small>{stateLabel(app)}</small>
                </span>
              </button>
            ))}
          </div>
        </aside>
        <section className="main-pane">
          {resourceView ? (
            <ResourcePoolView
              pools={pools.data}
              loading={pools.isLoading}
              error={pools.isError}
              retry={() => void pools.refetch()}
              onConfirm={(label, execute, detail) => setPending({ label, execute, detail })}
            />
          ) : selected ? (
            <AppWorkbench
              key={selected.name}
              app={selected}
              operation={operation}
              configuration={configuration}
              pools={configurationPools.data}
              configLoading={config.isLoading}
              configError={config.isError}
              onRetry={() => void config.refetch()}
              onAction={action}
              onRun={run}
              onConfirm={(label, execute, detail) => setPending({ label, execute, detail })}
            />
          ) : (
            <RegisterForm busy={register.isPending} onSubmit={(input) => register.mutate(input)} />
          )}
        </section>
        <aside className="activity-pane">
          <div className="pane-title">
            <span>操作とログ</span>
            <ScrollText size={17} />
          </div>
          <OperationView operation={operation} />
          <LogPane
            view={logView}
            service={logService}
            services={services}
            entries={logs}
            query={logQuery}
            onViewChange={(view) => {
              setLogView(view);
              setLogService(undefined);
            }}
            onServiceChange={setLogService}
            onQueryChange={setLogQuery}
          />
        </aside>
      </section>
      <dialog ref={confirmRef} className="confirm-dialog">
        <form method="dialog">
          <AlertTriangle size={26} />
          <h2>{pending?.label}</h2>
          <p>
            {pending?.detail ??
              "この操作は取り消せません。対象と影響を確認してから実行してください。"}
          </p>
          <div className="dialog-actions">
            <button>戻る</button>
            <button
              className="danger"
              onClick={(event) => {
                event.preventDefault();
                const task = pending;
                confirmRef.current?.close();
                setPending(undefined);
                if (task) void run(task.execute);
              }}
            >
              実行する
            </button>
          </div>
        </form>
      </dialog>
    </main>
  );
}

function RegisterForm({
  busy,
  onSubmit,
}: {
  busy: boolean;
  onSubmit: (input: CreateApplicationInput) => void;
}) {
  const [repositoryUrl, setRepositoryUrl] = useState("");
  const [ref, setRef] = useState("main");
  const [subdomain, setSubdomain] = useState("test-app");
  const [composeFile, setComposeFile] = useState("compose.yaml");
  const [branches, setBranches] = useState<string[]>([]);
  const [composeFiles, setComposeFiles] = useState<string[]>([]);
  const [overrideFiles, setOverrideFiles] = useState<string[]>([]);
  const [lookupState, setLookupState] = useState<"idle" | "loading" | "ready" | "error">("idle");
  const repositoryReady = /^https:\/\/github\.com\/[^/]+\/[^/]+/.test(repositoryUrl);
  useEffect(() => {
    const match = repositoryUrl.match(
      /^https:\/\/github\.com\/([^/]+)\/([^/]+?)(?:\.git)?(?:\/tree\/(.*))?\/?$/,
    );
    if (!match) {
      setBranches([]);
      setComposeFiles([]);
      setOverrideFiles([]);
      setLookupState("idle");
      return;
    }
    const [, owner, name, urlRef] = match;
    if (urlRef) setRef(decodeURIComponent(urlRef));
    setLookupState("loading");
    let active = true;
    void Promise.all([
      fetch(`https://api.github.com/repos/${owner}/${name}/branches?per_page=100`).then((r) =>
        r.ok ? r.json() : [],
      ),
      fetch(
        `https://api.github.com/repos/${owner}/${name}/contents?ref=${encodeURIComponent(urlRef || ref)}`,
      ).then((r) => (r.ok ? r.json() : [])),
    ])
      .then(([branchData, fileData]) => {
        if (!active) return;
        const nextBranches = Array.isArray(branchData)
          ? branchData.map((item: { name: string }) => item.name)
          : [];
        const names = Array.isArray(fileData)
          ? fileData
              .map((item: { name: string }) => item.name)
              .filter((name: string) => /compose.*\.(yaml|yml)$/.test(name))
          : [];
        const priority = [
          "compose.yaml",
          "compose.yml",
          "docker-compose.yaml",
          "docker-compose.yml",
        ];
        names.sort(
          (a: string, b: string) =>
            (priority.indexOf(a) < 0 ? 99 : priority.indexOf(a)) -
            (priority.indexOf(b) < 0 ? 99 : priority.indexOf(b)),
        );
        setBranches(nextBranches);
        setComposeFiles(names);
        setSubdomain(
          name
            .toLowerCase()
            .replace(/[^a-z0-9-]+/g, "-")
            .replace(/^-+|-+$/g, "")
            .slice(0, 62) || "app",
        );
        if (names.length && !names.includes(composeFile)) setComposeFile(names[0]);
        setOverrideFiles((current) =>
          current.filter((file) => names.includes(file) && file !== (names[0] ?? composeFile)),
        );
        setLookupState("ready");
      })
      .catch(() => {
        if (active) setLookupState("error");
      });
    return () => {
      active = false;
    };
  }, [repositoryUrl, ref]);
  return (
    <section className="empty-workbench">
      <div className="empty-icon">
        <CirclePlus />
      </div>
      <span className="eyebrow">最初のアプリ</span>
      <h2>台帳にアプリを登録する</h2>
      <p>GitHub リポジトリの Compose 定義を確認して、LAN 内の URL を準備します。</p>
      <form
        className="register-form"
        onSubmit={(event) => {
          event.preventDefault();
          onSubmit({
            repositoryUrl,
            ref,
            subdomain,
            ...(overrideFiles.length ? { overrideFiles } : {}),
            ...(composeFile !== "compose.yaml" ? { composeFile } : {}),
          });
        }}
      >
        <label>
          GitHub リポジトリ
          <input
            required
            type="url"
            placeholder="https://github.com/owner/repository"
            value={repositoryUrl}
            onChange={(e) => setRepositoryUrl(e.target.value)}
          />
        </label>
        <fieldset className="override-picker">
          <legend>overrideするComposeファイル（任意）</legend>
          <small className="field-hint">
            未選択のまま登録するとoverrideは適用されません。複数選択時は上から順に読み込みます。
          </small>
          {composeFiles.filter((file) => file !== composeFile).length ? (
            composeFiles
              .filter((file) => file !== composeFile)
              .map((file) => {
                const index = overrideFiles.indexOf(file);
                return (
                  <div className="override-row" key={file}>
                    <label className="override-check">
                      <input
                        type="checkbox"
                        checked={index >= 0}
                        onChange={(event) =>
                          setOverrideFiles((current) =>
                            event.target.checked
                              ? [...current, file]
                              : current.filter((item) => item !== file),
                          )
                        }
                      />{" "}
                      {file}
                    </label>
                    {index >= 0 && (
                      <span className="override-actions">
                        <button
                          type="button"
                          disabled={index === 0}
                          onClick={() =>
                            setOverrideFiles((current) => {
                              const next = [...current];
                              [next[index - 1], next[index]] = [next[index], next[index - 1]];
                              return next;
                            })
                          }
                        >
                          上へ
                        </button>
                        <button
                          type="button"
                          disabled={index === overrideFiles.length - 1}
                          onClick={() =>
                            setOverrideFiles((current) => {
                              const next = [...current];
                              [next[index], next[index + 1]] = [next[index + 1], next[index]];
                              return next;
                            })
                          }
                        >
                          下へ
                        </button>
                      </span>
                    )}
                  </div>
                );
              })
          ) : (
            <p className="field-hint">選択可能なoverride Composeファイルはありません。</p>
          )}
        </fieldset>
        <div className="form-row">
          <label>
            ブランチまたはタグ
            {branches.length ? (
              <select value={ref} onChange={(e) => setRef(e.target.value)}>
                {branches.map((branch) => (
                  <option key={branch}>{branch}</option>
                ))}
              </select>
            ) : (
              <input required value={ref} onChange={(e) => setRef(e.target.value)} />
            )}
          </label>
          <label>
            公開名
            <input
              required
              pattern="[a-z0-9][a-z0-9-]{0,61}"
              value={subdomain}
              onChange={(e) => setSubdomain(e.target.value)}
            />
          </label>
        </div>
        <label>
          使用するComposeファイル
          {repositoryReady ? (
            <select
              value={composeFile}
              onChange={(e) => {
                const next = e.target.value;
                setComposeFile(next);
                setOverrideFiles((current) => current.filter((file) => file !== next));
              }}
              disabled={lookupState === "loading"}
            >
              {(composeFiles.length
                ? composeFiles
                : ["compose.yaml", "compose.yml", "docker-compose.yaml", "docker-compose.yml"]
              ).map((file) => (
                <option key={file}>{file}</option>
              ))}
            </select>
          ) : (
            <input value={composeFile} readOnly aria-label="使用するComposeファイル" />
          )}
          <small className="field-hint">
            優先順位: compose.yaml → compose.yml → docker-compose.yaml → docker-compose.yml
          </small>
        </label>
        {lookupState === "loading" && <p className="field-hint">リポジトリを確認しています…</p>}
        {lookupState === "error" && (
          <p className="field-hint error-text">
            リポジトリ情報を取得できません。URLと公開設定を確認してください。
          </p>
        )}
        <button className="primary" disabled={busy}>
          {busy ? <LoaderCircle className="spin" size={18} /> : <CirclePlus size={18} />}
          検証して登録
        </button>
      </form>
    </section>
  );
}

function ResourcePoolView({
  pools,
  loading,
  error,
  retry,
  onConfirm,
}: {
  pools?: ResourcePools;
  loading: boolean;
  error: boolean;
  retry: () => void;
  onConfirm: (label: string, execute: () => Promise<{ name: string }>, detail?: string) => void;
}) {
  const [deviceName, setDeviceName] = useState("");
  const [candidate, setCandidate] = useState("");
  const [saving, setSaving] = useState(false);
  const [saveError, setSaveError] = useState("");
  const [showSystem, setShowSystem] = useState(false);
  const [deleteError, setDeleteError] = useState("");
  const visiblePools = useQuery({
    queryKey: ["resource-pools", showSystem],
    queryFn: () => api.resourcePools(showSystem),
  });
  const candidates = visiblePools.data?.physicalDevices ?? [];
  if (loading)
    return (
      <section className="empty-workbench">
        <h2>リソースプールを読み込んでいます</h2>
      </section>
    );
  if (error)
    return (
      <section className="empty-workbench">
        <h2>リソースプールを取得できません</h2>
        <button className="primary" onClick={retry}>
          再試行
        </button>
      </section>
    );
  const groups = [
    {
      title: "Volume",
      rows: pools?.volumes ?? [],
      note: "Composeの名前付きVolume。LWS所有として保持します。",
    },
    {
      title: "Network",
      rows: pools?.networks ?? [],
      note: "edge networkとCompose本来のネットワークを分けて表示します。",
    },
    {
      title: "Device",
      rows: pools?.devices ?? [],
      note: "安定識別子で登録し、現在の/dev pathは実行時に解決します。",
    },
  ];
  return (
    <section className="app-workbench resource-pool">
      <header className="app-heading">
        <div>
          <span className="eyebrow">LWS 所有リソース</span>
          <h2>リソースプール</h2>
        </div>
      </header>
      <p className="settings-note">
        Volume と Network
        はCompose定義から管理状態を集約します。Deviceは検出済みの物理デバイスをLWS名へ登録してからアプリ設定で選択します。
      </p>
      <section className="settings-section">
        <div className="section-heading">
          <h3>物理デバイスをLWSへ登録</h3>
        </div>
        <div className="device-register">
          <input
            value={deviceName}
            placeholder="LWSデバイス名"
            onChange={(e) => setDeviceName(e.target.value)}
          />
          <select value={candidate} onChange={(e) => setCandidate(e.target.value)}>
            <option value="">検出済みデバイスを選択</option>
            {candidates.map((item) => (
              <option key={`${item.stableId}:${item.currentPath}`} value={item.stableId}>
                {item.name} · {item.currentPath}
                {item.metadata.identity === "usb topology" ? "（serialなし）" : ""}
              </option>
            ))}
          </select>
          <button
            className="primary small"
            disabled={saving || !deviceName || !candidate}
            onClick={() => {
              setSaving(true);
              setSaveError("");
              void api
                .createPoolDevice(deviceName, candidate)
                .then(() => {
                  setDeviceName("");
                  setCandidate("");
                  retry();
                })
                .catch((e) => setSaveError(e instanceof ApiError ? e.message : "登録できません"))
                .finally(() => setSaving(false));
            }}
          >
            登録
          </button>
        </div>
        {saveError && <p className="settings-note">{saveError}</p>}
        <button
          className="retry-button"
          onClick={() => {
            const next = !showSystem;
            setShowSystem(next);
            setCandidate("");
            if (!next) void retry();
          }}
        >
          {showSystem ? "ユーザーデバイスだけを表示" : "システムデバイスを表示"}
        </button>
        {showSystem && (
          <p className="settings-note">
            システムデバイスにはCPU・ディスク・仮想端末などが含まれます。割り当てるとコンテナがホスト機能へ直接アクセスできるため、必要性を確認してください。
          </p>
        )}
      </section>
      {groups.map((group) => (
        <section className="settings-section" key={group.title}>
          <div className="section-heading">
            <h3>{group.title}</h3>
          </div>
          <p className="settings-note">{group.note}</p>
          <div className="pool-list">
            {group.rows.length ? (
              (group.title === "Network" ? ["edge", "compose"] : [undefined]).flatMap((kind) =>
                group.rows
                  .filter(
                    (row) =>
                      (!("kind" in row) ? kind === undefined : row.kind === kind) ||
                      kind === undefined,
                  )
                  .map((row, index) => (
                    <div className="pool-row" key={`${group.title}-${kind ?? "all"}-${index}`}>
                      <code>{"name" in row ? row.name : ""}</code>
                      <span>
                        {group.title === "Network" && (
                          <b className="pool-kind">
                            {"kind" in row && row.kind === "edge" ? "edge" : "Compose"}
                          </b>
                        )}
                        {"applicationName" in row
                          ? row.applicationName
                          : "currentPath" in row
                            ? `${row.status} · ${row.currentPath || "未接続"}`
                            : "managed"}
                      </span>
                      {group.title === "Volume" && "deletable" in row && (
                        <span className="pool-status">
                          <b className={row.inUse ? "is-used" : "is-free"}>
                            {row.inUse ? "使用中" : "未使用"}
                          </b>
                          <button
                            className="pool-delete"
                            disabled={!row.deletable}
                            title={row.inUse ? "使用中のVolumeは削除できません" : "Volumeを削除"}
                            onClick={() => {
                              setDeleteError("");
                              onConfirm(
                                "Volumeを削除しますか？",
                                () =>
                                  api
                                    .deleteResourcePoolVolume(row.name)
                                    .then(() => ({ name: row.name }))
                                    .then((result) => {
                                      void visiblePools.refetch();
                                      return result;
                                    })
                                    .catch((error) => {
                                      setDeleteError(
                                        error instanceof ApiError
                                          ? error.message
                                          : "Volumeを削除できません",
                                      );
                                      throw error;
                                    }),
                                `対象: ${row.name}\n${row.inUse ? "現在使用中のため削除できません。" : "未使用のLWS管理Volumeです。"}`,
                              );
                            }}
                          >
                            <Trash2 size={14} />
                            削除
                          </button>
                        </span>
                      )}
                      {"stableId" in row && <small>{row.stableId}</small>}
                    </div>
                  )),
              )
            ) : (
              <p className="settings-note">登録済みの{group.title}はありません。</p>
            )}
          </div>
          {deleteError && <p className="settings-note error-note">{deleteError}</p>}
        </section>
      ))}
    </section>
  );
}

function AppWorkbench({
  app,
  operation,
  configuration,
  pools,
  configLoading,
  configError,
  onRetry,
  onAction,
  onRun,
  onConfirm,
}: {
  app: Application;
  operation?: Operation;
  configuration?: Configuration;
  pools?: ResourcePools;
  configLoading: boolean;
  configError: boolean;
  onRetry: () => void;
  onAction: (kind: "start" | "stop" | "sync" | "rebuild" | "register") => void;
  onRun: (task: () => Promise<{ name: string }>) => void;
  onConfirm: (label: string, execute: () => Promise<{ name: string }>, detail?: string) => void;
}) {
  const [values, setValues] = useState<Record<string, { value: string; secret: boolean }>>({});
  const [removed, setRemoved] = useState<string[]>([]);
  const [newName, setNewName] = useState("");
  const [newValue, setNewValue] = useState("");
  const [newSecret, setNewSecret] = useState(false);
  const [deviceValues, setDeviceValues] = useState<Record<string, string>>({});
  const [publicService, setPublicService] = useState("");
  const [publicPort, setPublicPort] = useState(0);
  const variables = configuration?.variables ?? [];
  useEffect(() => {
    if (configuration) {
      setPublicService(configuration.publicService ?? configuration.manifestPublicService ?? "");
      setPublicPort(configuration.publicPort ?? configuration.manifestPublicPort ?? 0);
    }
  }, [configuration]);
  const rows = [
    ...variables
      .filter((item) => !removed.includes(item.name))
      .map((item) => ({
        ...item,
        value: values[item.name]?.value ?? item.value ?? item.defaultValue ?? "",
        edited: Boolean(values[item.name]),
      })),
    ...(newName
      ? [
          {
            name: newName,
            isSecret: newSecret,
            configured: false,
            required: false,
            hasDefault: false,
            value: newValue,
            edited: true,
          },
        ]
      : []),
  ];
  const disabled =
    app.reconciling || operation?.state === "queued" || operation?.state === "running";
  const unregistered = app.registrationState === "UNREGISTERED";
  const actionTitle = disabled ? "実行中の操作が完了するまで待ってください" : undefined;
  const save = () =>
    Object.fromEntries(
      rows
        .filter((row) => row.value || (row.isSecret && row.configured && !row.edited))
        .map((row) => [
          row.name,
          row.isSecret && row.configured && !row.edited
            ? { secret: true, keep: true }
            : { value: row.value, secret: row.isSecret },
        ]),
    );
  const bindings = (configuration?.devices ?? []).map((device) => ({
    service: device.service,
    targetPath: device.targetPath,
    deviceId: deviceValues[`${device.service}\x00${device.targetPath}`] ?? device.deviceId ?? "",
  }));
  const changed = rows.filter((row) => row.edited || !row.configured).map((row) => row.name);
  const changeDetail = `${changed.length ? `追加・更新: ${changed.join("、")}` : "追加・更新はありません"}${removed.length ? `\n削除: ${removed.join("、")}` : ""}\nsecret の現在値は表示・送信されず、未変更の値は保持されます。`;
  const publicArgs: [] | [string, number] = configuration?.webInterfaces?.length
    ? [publicService, publicPort]
    : [];
  const resourcePending = configLoading || (!configuration && !configError);
  const resourceError = configError && !configuration;
  const resourceText = (value: string) =>
    resourcePending ? "読み込み中" : resourceError ? "取得できません" : value;
  return (
    <section className="app-workbench">
      <header className="app-heading">
        <div>
          <span className="eyebrow">公開アプリ</span>
          <h2>{app.subdomain}</h2>
          <a href={publicUrl(app)} target="_blank" rel="noreferrer">
            {publicUrl(app)} <ExternalLink size={14} />
          </a>
        </div>
        <span className={`state-pill ${stateTone(app)}`}>{stateLabel(app)}</span>
      </header>
      <div className="action-strip" aria-label="アプリ操作">
        {unregistered ? (
          <button disabled={disabled} title={actionTitle} onClick={() => onAction("register")}>
            <CirclePlus size={16} />
            再登録
          </button>
        ) : (
          <>
            <button disabled={disabled} title={actionTitle} onClick={() => onAction("start")}>
              <Play size={16} />
              開始
            </button>
            {app.registrationState !== "CONFIGURING" && (
              <button disabled={disabled} title={actionTitle} onClick={() => onAction("stop")}>
                <Square size={16} />
                停止
              </button>
            )}
            <button disabled={disabled} title={actionTitle} onClick={() => onAction("sync")}>
              <RotateCw size={16} />
              同期
            </button>
            <button disabled={disabled} title={actionTitle} onClick={() => onAction("rebuild")}>
              <Settings2 size={16} />
              再構成
            </button>
            <button
              disabled={disabled}
              title={actionTitle}
              onClick={() => onConfirm("登録を解除する", () => api.unregister(app))}
            >
              <FileCog size={16} />
              登録解除
            </button>
          </>
        )}
        <button
          className="danger-outline"
          disabled={disabled}
          title={actionTitle}
          onClick={() => onConfirm("アプリと保存データを完全に削除する", () => api.purge(app))}
        >
          <Trash2 size={16} />
          完全削除
        </button>
      </div>
      <section className="details-grid">
        <div>
          <span>リポジトリ</span>
          <code>{app.repositoryUrl}</code>
        </div>
        <div>
          <span>参照</span>
          <code>{app.ref}</code>
        </div>
        <div>
          <span>Composeファイル</span>
          <code>{app.composeFile ?? "compose.yaml"}</code>
        </div>
        <div>
          <span>実行状態</span>
          <strong>{observedStateLabel(app.observedState)}</strong>
        </div>
        <div>
          <span>登録状態</span>
          <strong>{registrationStateLabel(app.registrationState)}</strong>
        </div>
        {app.latestError && (
          <div>
            <span>異常内容</span>
            <strong>{app.latestError}</strong>
          </div>
        )}
        <div>
          <span>最終確認</span>
          <time>{new Date(app.observedAt).toLocaleString("ja-JP")}</time>
        </div>
      </section>
      <section className="settings-section resource-section" aria-label="Dockerリソース">
        <div className="section-heading">
          <div>
            <span className="eyebrow">Docker リソース</span>
            <h3>保持と分離</h3>
          </div>
          {resourceError && (
            <button className="retry-button" onClick={onRetry}>
              再試行
            </button>
          )}
        </div>
        <div className="resource-grid">
          <article>
            <Database size={17} />
            <div>
              <strong>Volume</strong>
              <p>
                {resourceText(
                  configuration?.volumes?.length
                    ? configuration.volumes.map((volume) => volume.name).join("、")
                    : "名前付きVolumeはありません",
                )}
              </p>
              <small>
                {resourceError
                  ? "設定を取得できません。再試行してください。"
                  : "停止・再構成・登録解除でも保持され、完全削除時だけ削除されます。"}
              </small>
            </div>
          </article>
          <article>
            <Network size={17} />
            <div>
              <strong>Network</strong>
              <p>
                <code>{resourceText(configuration?.network?.name ?? "確認中")}</code>
              </p>
              <small>
                {resourceError
                  ? "設定を取得できません。再試行してください。"
                  : (configuration?.network?.purpose ?? "公開経路を分離しています")}
              </small>
            </div>
          </article>
          <article>
            <Usb size={17} />
            <div>
              <strong>デバイス</strong>
              <p>
                {resourceText(
                  configuration?.devices?.length
                    ? `${configuration.devices.length} 件の割り当て`
                    : "デバイスは不要",
                )}
              </p>
              <small>
                {resourceError
                  ? "設定を取得できません。再試行してください。"
                  : configuration?.devices?.length
                    ? "設定レイヤーでLWSデバイスを選択します。"
                    : "Composeはデバイスを要求していません。"}
              </small>
            </div>
          </article>
        </div>
      </section>
      <section className="settings-section">
        <div className="section-heading">
          <div>
            <span className="eyebrow">環境設定</span>
            <h3>変数</h3>
          </div>
          <button
            className="primary small"
            disabled={disabled || unregistered || configLoading || configError}
            onClick={() =>
              onConfirm(
                app.registrationState === "CONFIGURING" ? "アプリを登録する" : "環境設定を更新する",
                () =>
                  api.saveConfiguration(
                    app,
                    save(),
                    bindings.filter((binding) => binding.deviceId),
                    ...publicArgs,
                  ),
                changeDetail,
              )
            }
          >
            {app.registrationState === "CONFIGURING" ? "登録" : "保存"}
          </button>
        </div>
        {configLoading ? (
          <p>設定を読み込んでいます</p>
        ) : configError ? (
          <p className="settings-note">設定を取得できません。再試行してください。</p>
        ) : (
          <>
            <p className="settings-note">
              Compose が参照する変数を管理します。secret
              の現在値は表示せず、未変更なら安全に保持します。
            </p>
            {configuration?.webInterfaces?.length ? (
              <div className="interface-setting">
                <label>
                  公開するWebインターフェース
                  <select
                    disabled={disabled || unregistered}
                    value={`${publicService}:${publicPort}`}
                    onChange={(event) => {
                      const [service, port] = event.target.value.split(":");
                      setPublicService(service);
                      setPublicPort(Number(port));
                    }}
                  >
                    {(configuration?.webInterfaces ?? []).map((iface) => (
                      <option
                        key={`${iface.service}:${iface.port}`}
                        value={`${iface.service}:${iface.port}`}
                      >
                        {iface.service}:{iface.port}
                        {iface.service === configuration?.manifestPublicService &&
                        iface.port === configuration?.manifestPublicPort
                          ? "（manifestの既定値）"
                          : ""}
                      </option>
                    ))}
                  </select>
                </label>
                <small>
                  manifestの公開先が初期選択されています。開発用サービスへ切り替える場合だけ変更してください。
                </small>
              </div>
            ) : null}
            {configuration?.devices?.length ? (
              <div className="device-binding-list">
                {configuration.devices.map((device) => {
                  const key = `${device.service}\x00${device.targetPath}`;
                  const selected = deviceValues[key] ?? device.deviceId ?? "";
                  return (
                    <label className="device-binding-row" key={key}>
                      <code>
                        {device.service}: {device.targetPath}
                      </code>
                      <small>
                        Compose hint: {device.sourceHint} · {device.permissions}
                      </small>
                      <select
                        disabled={disabled || unregistered}
                        value={selected}
                        onChange={(event) =>
                          setDeviceValues((current) => ({
                            ...current,
                            [key]: event.target.value,
                          }))
                        }
                      >
                        <option value="">LWSデバイスを選択</option>
                        {pools?.devices
                          .filter((pool) => pool.status === "connected")
                          .map((pool) => (
                            <option key={pool.id} value={pool.id}>
                              {pool.name} · {pool.currentPath}
                            </option>
                          ))}
                      </select>
                    </label>
                  );
                })}
              </div>
            ) : null}
            <div className="variable-list">
              {rows.map((row) => (
                <div key={row.name} className="variable-row">
                  <code>{row.name}</code>
                  <input
                    aria-label={row.name}
                    disabled={disabled || unregistered}
                    type={row.isSecret ? "password" : "text"}
                    value={row.value}
                    placeholder={
                      row.isSecret && row.configured && !row.edited
                        ? "設定済み（変更する場合のみ入力）"
                        : row.isSecret
                          ? "secret の値を入力"
                          : "値を入力"
                    }
                    onChange={(event) =>
                      setValues((current) => ({
                        ...current,
                        [row.name]: {
                          value: event.target.value,
                          secret: row.isSecret,
                        },
                      }))
                    }
                  />
                  <small>
                    {row.required
                      ? "必須"
                      : row.hasDefault
                        ? "既定値あり"
                        : row.isSecret
                          ? "secret"
                          : "任意"}
                  </small>
                  <button
                    className="variable-remove"
                    disabled={disabled || unregistered}
                    aria-label={`${row.name} を削除`}
                    title={`${row.name} を削除`}
                    onClick={() => {
                      setRemoved((current) => [...current, row.name]);
                      setValues((current) => {
                        const next = { ...current };
                        delete next[row.name];
                        return next;
                      });
                    }}
                  >
                    <X size={15} />
                  </button>
                </div>
              ))}
            </div>
            <div className="add-variable">
              <input
                disabled={disabled || unregistered}
                placeholder="変数名"
                value={newName}
                onChange={(e) => setNewName(e.target.value.toUpperCase())}
              />
              <input
                disabled={disabled || unregistered}
                placeholder="値"
                value={newValue}
                onChange={(e) => setNewValue(e.target.value)}
              />
              <label>
                <input
                  disabled={disabled || unregistered}
                  type="checkbox"
                  checked={newSecret}
                  onChange={(e) => setNewSecret(e.target.checked)}
                />
                secret
              </label>
            </div>
            {configuration?.effectiveCompose ? (
              <details className="override-preview">
                <summary>マージ後の完全なComposeを確認</summary>
                <pre>{configuration.effectiveCompose}</pre>
              </details>
            ) : null}
          </>
        )}
      </section>
    </section>
  );
}

function OperationView({ operation }: { operation?: Operation }) {
  return (
    <section className="operation" aria-live="polite">
      <span className="eyebrow">現在の操作</span>
      {operation ? (
        <>
          <strong>
            {operationLabel(operation.kind)}:{" "}
            {operation.state === "queued"
              ? "開始待ち"
              : operation.state === "running"
                ? "実行中"
                : operation.state === "succeeded"
                  ? "完了"
                  : operation.state === "cancelled"
                    ? "中止"
                    : "失敗"}
          </strong>
          <small>{operation.name}</small>
          {operation.phase && <small>段階: {phaseLabel(operation.phase)}</small>}
          <time>
            {operation.createdAt
              ? `開始: ${new Date(operation.createdAt).toLocaleString("ja-JP")}`
              : "開始時刻を確認中です"}
          </time>
          <p>
            {operation.errorMessage ||
              operation.displayMessage ||
              (operation.state === "running" || operation.state === "queued"
                ? "進捗を確認しています。"
                : "")}
          </p>
        </>
      ) : (
        <p>実行中の操作はありません。</p>
      )}
    </section>
  );
}

function LogPane({
  view,
  service,
  services,
  entries,
  query,
  onViewChange,
  onServiceChange,
  onQueryChange,
}: {
  view: LogView;
  service?: string;
  services: string[];
  entries: LogEntry[];
  query: LogQuery;
  onViewChange: (view: LogView) => void;
  onServiceChange: (service?: string) => void;
  onQueryChange: (query: LogQuery) => void;
}) {
  const labels: Record<LogView, string> = {
    task: "タスクログ",
    application: "アプリログ",
    related: "関連ログ",
  };
  const [draft, setDraft] = useState({
    limit: String(query.limit ?? 200),
    startAt: query.startAt?.slice(0, 16) ?? "",
    endAt: query.endAt?.slice(0, 16) ?? "",
  });
  const displayEntries = groupLogEntries(entries);
  const applyQuery = () =>
    onQueryChange({
      limit: Math.max(1, Math.min(500, Number(draft.limit) || 200)),
      startAt: draft.startAt ? new Date(draft.startAt).toISOString() : undefined,
      endAt: draft.endAt ? new Date(draft.endAt).toISOString() : undefined,
    });
  return (
    <section className="log-pane" aria-label="アプリ運用ログ">
      <div className="log-tabs" role="tablist" aria-label="ログ種別">
        {(Object.keys(labels) as LogView[]).map((item) => (
          <button
            key={item}
            role="tab"
            aria-selected={view === item}
            onClick={() => onViewChange(item)}
          >
            {labels[item]}
          </button>
        ))}
      </div>
      <div className="log-filters">
        <label>
          件数
          <input
            type="number"
            min="1"
            max="500"
            value={draft.limit}
            onChange={(event) => setDraft({ ...draft, limit: event.target.value })}
          />
        </label>
        <label>
          開始日時
          <input
            type="datetime-local"
            value={draft.startAt}
            onChange={(event) => setDraft({ ...draft, startAt: event.target.value })}
          />
        </label>
        <label>
          終了日時
          <input
            type="datetime-local"
            value={draft.endAt}
            onChange={(event) => setDraft({ ...draft, endAt: event.target.value })}
          />
        </label>
        <button onClick={applyQuery}>再読込</button>
      </div>
      {view === "application" && (
        <label className="service-select">
          対象コンテナ
          <select
            value={service ?? ""}
            onChange={(event) => onServiceChange(event.target.value || undefined)}
          >
            <option value="">全てのコンテナ</option>
            {services.map((item) => (
              <option key={item} value={item}>
                {item}
              </option>
            ))}
          </select>
        </label>
      )}
      <ol className="log-list" aria-live="polite">
        {displayEntries.length ? (
          displayEntries.map((item) => <LogRow key={item.entry.id} item={item} view={view} />)
        ) : (
          <li className="log-empty">{labels[view]}はまだありません。</li>
        )}
      </ol>
    </section>
  );
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
          if (
            candidate.component !== entry.component ||
            candidate.service !== entry.service ||
            candidate.containerName !== entry.containerName ||
            candidate.level !== entry.level ||
            logTask(candidate.message) !== task
          )
            break;
          lines.push(logBody(candidate.message));
        }
        try {
          json = JSON.stringify(JSON.parse(lines.join("\n")));
          index = next;
          break;
        } catch {
          /* 完結したJSONになるまで次の連続行を確認する。 */
        }
      }
    }
    groups.push({
      entry,
      message: lines.join("\n"),
      lineCount: lines.length,
      json,
    });
  }
  return groups;
}

function logTask(message: string) {
  return message.match(/^\[([^\]]+)\]\s*/)?.[1] ?? "";
}
function logBody(message: string) {
  return message.replace(/^\[[^\]]+\]\s*/, "");
}

function LogRow({ item, view }: { item: DisplayLog; view: LogView }) {
  const { entry } = item;
  const subject =
    view === "task"
      ? logTask(entry.message) || "操作"
      : view === "application"
        ? (entry.service ?? "container")
        : entry.component;
  return (
    <li className={`log-row level-${entry.level}`}>
      <div className="log-meta">
        <time dateTime={entry.occurredAt}>
          {new Date(entry.occurredAt).toLocaleTimeString("ja-JP")}
        </time>
        <span className="log-level">{entry.level}</span>
        <span className="log-subject">{subject}</span>
      </div>
      {item.json ? (
        <details className="log-json">
          <summary>
            構造化ログ <small>{item.lineCount}行</small>
          </summary>
          <code>{item.json}</code>
        </details>
      ) : (
        <p>{item.message.replace(/^\[[^\]]+\]\s*/, "")}</p>
      )}
    </li>
  );
}
