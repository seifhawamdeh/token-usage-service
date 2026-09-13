import { useEffect, useState } from "react";
import { getJSON, buildUrl } from "./lib/api";
import type { Summary, Vendor, Model, Project, Daily, Remote, Machine, Snapshot, Cwd } from "./types";
import { fmt } from "./lib/utils";
import {
  BarChart,
  Bar,
  XAxis,
  YAxis,
  CartesianGrid,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
import { LayoutDashboard, Settings2, RefreshCw, Home, Calendar, Settings, Server } from "lucide-react";

export default function App() {
  const [machines, setMachines] = useState<Machine[]>([]);
  const [projects, setProjects] = useState<Project[]>([]);
  const [vendors, setVendors] = useState<Vendor[]>([]);
  const [models, setModels] = useState<Model[]>([]);
  const [daily, setDaily] = useState<Daily[]>([]);
  const [remotes, setRemotes] = useState<Remote[]>([]);
  const [cwds, setCwds] = useState<Cwd[]>([]);
  const [summary, setSummary] = useState<Summary | null>(null);
  const [snaps, setSnaps] = useState<Snapshot[]>([]);
  const [dailySnaps, setDailySnaps] = useState<Snapshot[]>([]);

  const [filterMachine, setFilterMachine] = useState("");
  const [filterProject, setFilterProject] = useState("");
  const [filterVendor, setFilterVendor] = useState("");
  
  const [activeTab, setActiveTab] = useState("home");
  const [selectedDay, setSelectedDay] = useState<string>("");

  const [error, setError] = useState("");

  const loadAll = async () => {
    try {
      setError("");
      
      const m = await getJSON("/api/machines");
      setMachines(m);
      
      const p = await getJSON("/api/projects");
      setProjects(p);

      const params = { machine: filterMachine, project: filterProject };
      const [sum, v, mod, bp, d, r, c] = await Promise.all([
        getJSON(buildUrl("/api/summary", params)),
        getJSON(buildUrl("/api/by-vendor", params)),
        getJSON(buildUrl("/api/by-model", { ...params, limit: "40" })),
        getJSON(buildUrl("/api/by-project", params)),
        getJSON(buildUrl("/api/daily", { ...params, days: "45" })),
        getJSON(buildUrl("/api/remotes", params)),
        getJSON(buildUrl("/api/cwds", params)),
      ]);

      setSummary(sum);
      setVendors(v);
      setModels(mod);
      setProjects(bp);
      setDaily(d);
      setRemotes(r);
      setCwds(c);
      
      if (!selectedDay && d.length > 0) {
        setSelectedDay(d[d.length - 1].day);
      }

      await loadSnaps();
    } catch (err: any) {
      setError(err.message);
    }
  };

  const loadSnaps = async () => {
    const params = {
      machine: filterMachine,
      project: filterProject,
      vendor: filterVendor,
      limit: "50",
    };
    try {
      const s = await getJSON(buildUrl("/api/snapshots", params));
      setSnaps(s);
    } catch (err: any) {
      setError(err.message);
    }
  };

  useEffect(() => {
    loadAll();
  }, [filterMachine, filterProject]);

  useEffect(() => {
    loadSnaps();
  }, [filterVendor]);

  useEffect(() => {
    if (!selectedDay) return;
    const loadDailySnaps = async () => {
      const params = {
        machine: filterMachine,
        project: filterProject,
        day: selectedDay,
        limit: "500",
      };
      try {
        const s = await getJSON(buildUrl("/api/snapshots", params));
        setDailySnaps(s);
      } catch (err: any) {
        setError(err.message);
      }
    };
    loadDailySnaps();
  }, [selectedDay, filterMachine, filterProject]);

  const handleSaveProject = async (remote_url: string, project: string) => {
    try {
      const res = await fetch("/api/remotes/map", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ remote_url, project }),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || "failed");
      }
      loadAll();
    } catch (err: any) {
      alert(err.message);
    }
  };

  const handleSaveCwd = async (hostId: string, cwd: string, project: string) => {
    try {
      const res = await fetch("/api/cwds/map", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ host_id: hostId, cwd, project }),
      });
      if (!res.ok) {
        const err = await res.json();
        throw new Error(err.error || "failed");
      }
      loadAll();
    } catch (err: any) {
      alert(err.message);
    }
  };

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col md:flex-row">
      {/* Sidebar */}
      <aside className="w-full md:w-64 bg-white border-r border-gray-200 flex-shrink-0">
        <div className="p-6 flex items-center gap-2 text-indigo-600 border-b border-gray-200">
          <LayoutDashboard className="w-6 h-6" />
          <h1 className="text-xl font-bold tracking-tight text-gray-900">Token Usage</h1>
        </div>
        <nav className="p-4 space-y-1">
          <button
            onClick={() => setActiveTab("home")}
            className={`w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
              activeTab === "home" ? "bg-indigo-50 text-indigo-700" : "text-gray-700 hover:bg-gray-100"
            }`}
          >
            <Home className="w-4 h-4" />
            Home
          </button>
          <button
            onClick={() => setActiveTab("daily")}
            className={`w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
              activeTab === "daily" ? "bg-indigo-50 text-indigo-700" : "text-gray-700 hover:bg-gray-100"
            }`}
          >
            <Calendar className="w-4 h-4" />
            Daily Dashboard
          </button>
          <button
            onClick={() => setActiveTab("machines")}
            className={`w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
              activeTab === "machines" ? "bg-indigo-50 text-indigo-700" : "text-gray-700 hover:bg-gray-100"
            }`}
          >
            <Server className="w-4 h-4" />
            Machines
          </button>
          <button
            onClick={() => setActiveTab("configs")}
            className={`w-full flex items-center gap-3 px-3 py-2 rounded-md text-sm font-medium transition-colors ${
              activeTab === "configs" ? "bg-indigo-50 text-indigo-700" : "text-gray-700 hover:bg-gray-100"
            }`}
          >
            <Settings className="w-4 h-4" />
            Configs
          </button>
        </nav>
      </aside>

      <div className="flex-1 flex flex-col min-w-0">
        <header className="bg-white border-b border-gray-200 sticky top-0 z-10 shadow-sm">
          <div className="px-6 py-4 flex flex-wrap items-center justify-end gap-3">
            <div className="flex items-center bg-gray-100 rounded-md px-3 py-1">
              <Settings2 className="w-4 h-4 text-gray-500 mr-2" />
              <select
                className="bg-transparent border-none text-sm font-medium text-gray-700 focus:ring-0 cursor-pointer outline-none"
                value={filterMachine}
                onChange={(e) => setFilterMachine(e.target.value)}
              >
                <option value="">All machines</option>
                {machines.map((m) => (
                  <option key={m.machine} value={m.machine}>
                    {m.machine} ({fmt.int(m.sources)})
                  </option>
                ))}
              </select>
            </div>

            <select
              className="bg-gray-100 border-none rounded-md px-3 py-1 text-sm font-medium text-gray-700 focus:ring-0 cursor-pointer outline-none"
              value={filterProject}
              onChange={(e) => setFilterProject(e.target.value)}
            >
              <option value="">All projects</option>
              {projects.map((p) => (
                <option key={p.project} value={p.project}>
                  {p.project} ({fmt.int(p.sources)})
                </option>
              ))}
            </select>

            <button
              onClick={loadAll}
              className="p-1.5 text-gray-500 hover:text-indigo-600 transition-colors bg-gray-100 hover:bg-indigo-50 rounded-md"
              title="Refresh data"
            >
              <RefreshCw className="w-4 h-4" />
            </button>
          </div>
        </header>

        <main className="flex-1 overflow-y-auto p-6 space-y-8">
          {error && (
            <div className="bg-red-50 border-l-4 border-red-500 p-4 rounded-md">
              <p className="text-sm text-red-700">{error}</p>
            </div>
          )}

          {activeTab === "home" && summary && (
            <div className="space-y-8 animate-in fade-in duration-300">
              <div className="grid grid-cols-2 md:grid-cols-3 lg:grid-cols-6 gap-4">
                <KpiCard label="Sources" value={fmt.int(summary.sources)} />
                <KpiCard label="Rated USD" value={fmt.usd(summary.rated_cost_usd)} className="text-indigo-600 font-bold" />
                <KpiCard label="Input tokens" value={fmt.compact(summary.tokens_input)} />
                <KpiCard label="Output tokens" value={fmt.compact(summary.tokens_output)} />
                <KpiCard label="Cache read" value={fmt.compact(summary.tokens_cache_read)} />
                <KpiCard label="Cache write 5m/1h" value={`${fmt.compact(summary.tokens_cache_write_5m)} / ${fmt.compact(summary.tokens_cache_write_1h)}`} />
              </div>

              <div className="grid grid-cols-1 lg:grid-cols-3 gap-8">
                <div className="lg:col-span-1 bg-white rounded-xl shadow-sm border border-gray-200 p-6 flex flex-col">
                  <h2 className="text-lg font-semibold text-gray-900 mb-6">Vendor Breakdown</h2>
                  <div className="space-y-4 flex-1">
                    {vendors.map((v) => {
                      const max = Math.max(...vendors.map((r) => r.rated_cost_usd || 0), 1);
                      const pct = ((v.rated_cost_usd || 0) / max) * 100;
                      return (
                        <div key={v.vendor} className="flex items-center gap-3 text-sm">
                          <div className="w-32 font-medium text-gray-700 truncate" title={v.vendor}>{v.vendor}</div>
                          <div className="flex-1 h-2.5 bg-gray-100 rounded-full overflow-hidden">
                            <div className="h-full bg-indigo-500 rounded-full" style={{ width: `${pct}%` }} />
                          </div>
                          <div className="w-24 text-right text-gray-500">{fmt.usd(v.rated_cost_usd)}</div>
                        </div>
                      );
                    })}
                  </div>
                </div>

                <div className="lg:col-span-2">
                  <Card title="Projects">
                    <Table headers={["Project", "Sources", "In", "Out", "Rated USD"]}>
                      {projects.map((p) => (
                        <tr key={p.project}>
                          <td className="font-medium text-gray-900">{p.project}</td>
                          <td className="text-right text-gray-500">{fmt.int(p.sources)}</td>
                          <td className="text-right text-gray-500">{fmt.compact(p.tokens_input)}</td>
                          <td className="text-right text-gray-500">{fmt.compact(p.tokens_output)}</td>
                          <td className="text-right text-gray-900 font-medium">{fmt.usd(p.rated_cost_usd)}</td>
                        </tr>
                      ))}
                    </Table>
                  </Card>
                </div>
              </div>

              <div className="grid grid-cols-1 gap-8">
                <Card title="Models (Top 40)">
                  <Table headers={["Model", "Vendor", "Sources", "In", "Out", "Rated USD"]}>
                    {models.map((m) => (
                      <tr key={`${m.model}-${m.vendor}`}>
                        <td className="font-medium text-gray-900">{m.model}</td>
                        <td className="text-gray-500">{m.vendor}</td>
                        <td className="text-right text-gray-500">{fmt.int(m.sources)}</td>
                        <td className="text-right text-gray-500">{fmt.compact(m.tokens_input)}</td>
                        <td className="text-right text-gray-500">{fmt.compact(m.tokens_output)}</td>
                        <td className="text-right text-gray-900 font-medium">{fmt.usd(m.rated_cost_usd)}</td>
                      </tr>
                    ))}
                  </Table>
                </Card>
              </div>

              <Card title="Latest Snapshots (All Time)">
                <div className="mb-4">
                  <select
                    className="text-sm border-gray-300 rounded-md shadow-sm focus:border-indigo-500 focus:ring-indigo-500 py-1.5 pl-3 pr-8 border"
                    value={filterVendor}
                    onChange={(e) => setFilterVendor(e.target.value)}
                  >
                    <option value="">All vendors</option>
                    {vendors.map((v) => (
                      <option key={v.vendor} value={v.vendor}>{v.vendor}</option>
                    ))}
                  </select>
                </div>
                <div className="overflow-x-auto">
                  <Table headers={["When", "Machine", "Project", "Vendor", "Model", "In", "Out", "CR / CW", "CW 5m/1h", "Rated USD", "Path"]}>
                    {snaps.slice(0, 50).map((s) => (
                      <tr key={s.source_id}>
                        <td className="text-gray-500 whitespace-nowrap">{fmt.when(s.last_event_at || s.started_at || s.ingested_at)}</td>
                        <td className="text-gray-900">{s.machine}</td>
                        <td className="text-gray-900">{s.project}</td>
                        <td className="text-gray-900">{s.vendor}</td>
                        <td className="text-gray-500">{s.model || "—"}</td>
                        <td className="text-right text-gray-500">{fmt.compact(s.tokens_input)}</td>
                        <td className="text-right text-gray-500">{fmt.compact(s.tokens_output)}</td>
                        <td className="text-right text-gray-500">{fmt.compact(s.tokens_cache_read)} / {fmt.compact(s.tokens_cache_write)}</td>
                        <td className="text-right text-gray-500">{fmt.compact(s.tokens_cache_write_5m)} / {fmt.compact(s.tokens_cache_write_1h)}</td>
                        <td className="text-right text-gray-900 font-medium">{fmt.usd(s.rated_cost_usd)}</td>
                        <td className="text-gray-400 text-xs truncate max-w-[200px]" title={s.source_path}>{s.source_path}</td>
                      </tr>
                    ))}
                  </Table>
                </div>
              </Card>
            </div>
          )}

          {activeTab === "daily" && (
            <div className="space-y-8 animate-in fade-in duration-300">
              <div className="bg-white rounded-xl shadow-sm border border-gray-200 p-6">
                <div className="flex flex-col sm:flex-row items-center justify-between mb-6 gap-4">
                  <h2 className="text-lg font-semibold text-gray-900">Daily Rated Cost (USD)</h2>
                  <div className="flex items-center gap-3">
                    <label className="text-sm font-medium text-gray-700">Select Day:</label>
                    <input 
                      type="date" 
                      className="text-sm border-gray-300 rounded-md shadow-sm focus:border-indigo-500 focus:ring-indigo-500 px-3 py-1.5 border"
                      value={selectedDay}
                      onChange={(e) => setSelectedDay(e.target.value)}
                    />
                  </div>
                </div>
                <div className="h-64 w-full">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart data={daily}>
                      <CartesianGrid strokeDasharray="3 3" vertical={false} stroke="#e5e7eb" />
                      <XAxis dataKey="day" tickFormatter={fmt.day} axisLine={false} tickLine={false} tick={{fill: '#6b7280', fontSize: 12, cursor: 'pointer'}} onClick={(data: any) => { if (data && data.value) setSelectedDay(data.value); }} />
                      <YAxis axisLine={false} tickLine={false} tickFormatter={(v: number) => `$${v}`} tick={{fill: '#6b7280', fontSize: 12}} />
                      <Tooltip formatter={(value: any) => fmt.usd(Number(value))} labelFormatter={(label: any) => fmt.day(String(label))} cursor={{fill: '#f3f4f6'}} />
                      <Bar dataKey="rated_cost_usd" fill="#6366f1" radius={[4, 4, 0, 0]} className="cursor-pointer" onClick={(data: any) => { if (data && data.day) setSelectedDay(data.day); }} />
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>

              {selectedDay && (
                <Card title={`Sessions on ${selectedDay}`}>
                  <div className="overflow-x-auto">
                    <Table headers={["When", "Machine", "Project", "Vendor", "Model", "In", "Out", "Rated USD"]}>
                      {dailySnaps.length === 0 ? (
                        <tr>
                          <td colSpan={8} className="text-center py-4 text-gray-500">No sessions recorded on this day.</td>
                        </tr>
                      ) : (
                        dailySnaps.map((s) => (
                          <tr key={s.source_id}>
                            <td className="text-gray-500 whitespace-nowrap">{fmt.when(s.last_event_at || s.started_at || s.ingested_at)}</td>
                            <td className="text-gray-900">{s.machine}</td>
                            <td className="text-gray-900">{s.project}</td>
                            <td className="text-gray-900">{s.vendor}</td>
                            <td className="text-gray-500">{s.model || "—"}</td>
                            <td className="text-right text-gray-500">{fmt.compact(s.tokens_input)}</td>
                            <td className="text-right text-gray-500">{fmt.compact(s.tokens_output)}</td>
                            <td className="text-right text-gray-900 font-medium">{fmt.usd(s.rated_cost_usd)}</td>
                          </tr>
                        ))
                      )}
                    </Table>
                  </div>
                </Card>
              )}
            </div>
          )}

          {activeTab === "machines" && (
            <div className="space-y-8 animate-in fade-in duration-300">
              <Card title="Machines">
                <Table headers={["Machine", "Sources", "Last Sync"]}>
                  {machines.map((m) => (
                    <tr key={m.machine}>
                      <td className="font-medium text-gray-900">{m.machine}</td>
                      <td className="text-right text-gray-500">{fmt.int(m.sources)}</td>
                      <td className="text-gray-500">{fmt.when(m.last_sync_at)}</td>
                    </tr>
                  ))}
                </Table>
              </Card>
            </div>
          )}

          {activeTab === "configs" && (
            <div className="space-y-8 animate-in fade-in duration-300">
              <Card title="Path to Project Remotes">
                <Table headers={["Remote URL", "Snapshots", "Project Mapping", "Action"]}>
                  {remotes.map((r) => (
                    <tr key={r.remote_url}>
                      <td className="text-gray-900 break-all">{r.remote_url}</td>
                      <td className="text-right text-gray-500">{fmt.int(r.snapshots)}</td>
                      <td>
                        <input
                          type="text"
                          defaultValue={r.project || ""}
                          className="w-full text-sm border-gray-300 rounded-md shadow-sm focus:border-indigo-500 focus:ring-indigo-500 px-3 py-1.5 border"
                          placeholder="Project name"
                          id={`remote-${r.remote_url}`}
                        />
                      </td>
                      <td className="w-24">
                        <button
                          onClick={() => {
                            const input = document.getElementById(`remote-${r.remote_url}`) as HTMLInputElement;
                            if (input) handleSaveProject(r.remote_url, input.value.trim());
                          }}
                          className="w-full inline-flex justify-center items-center px-3 py-1.5 border border-transparent text-sm font-medium rounded shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                        >
                          Save
                        </button>
                      </td>
                    </tr>
                  ))}
                </Table>
              </Card>

              <Card title="Work Directory Mapping">
                <Table headers={["Work Directory", "Snapshots", "Project Mapping", "Action"]}>
                  {cwds.map((c) => (
                    <tr key={`${c.host_id}:${c.cwd}`}>
                      <td className="text-gray-900 break-all">{c.cwd}</td>
                      <td className="text-right text-gray-500">{fmt.int(c.snapshots)}</td>
                      <td>
                        <input
                          type="text"
                          defaultValue={c.project || ""}
                          className="w-full text-sm border-gray-300 rounded-md shadow-sm focus:border-indigo-500 focus:ring-indigo-500 px-3 py-1.5 border"
                          placeholder="Project name"
                          id={`cwd-${c.host_id}-${c.cwd}`}
                        />
                      </td>
                      <td className="w-24">
                        <button
                          onClick={() => {
                            const input = document.getElementById(`cwd-${c.host_id}-${c.cwd}`) as HTMLInputElement;
                            if (input) handleSaveCwd(c.host_id, c.cwd, input.value.trim());
                          }}
                          className="w-full inline-flex justify-center items-center px-3 py-1.5 border border-transparent text-sm font-medium rounded shadow-sm text-white bg-indigo-600 hover:bg-indigo-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-indigo-500"
                        >
                          Save
                        </button>
                      </td>
                    </tr>
                  ))}
                </Table>
              </Card>
            </div>
          )}
        </main>
      </div>
    </div>
  );
}

function KpiCard({ label, value, className }: { label: string; value: string; className?: string }) {
  return (
    <div className="bg-white p-4 rounded-xl shadow-sm border border-gray-200">
      <p className="text-sm font-medium text-gray-500 truncate">{label}</p>
      <p className={`mt-1 text-2xl font-semibold text-gray-900 ${className || ""}`}>{value}</p>
    </div>
  );
}

function Card({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div className="bg-white rounded-xl shadow-sm border border-gray-200 overflow-hidden flex flex-col">
      <div className="px-6 py-5 border-b border-gray-200">
        <h2 className="text-lg font-semibold text-gray-900">{title}</h2>
      </div>
      <div className="p-6 flex-1 overflow-auto">{children}</div>
    </div>
  );
}

function Table({ headers, children }: { headers: string[]; children: React.ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="min-w-full divide-y divide-gray-200">
        <thead>
          <tr>
            {headers.map((h, i) => (
              <th
                key={i}
                className={`px-3 py-3 text-left text-xs font-medium text-gray-500 uppercase tracking-wider whitespace-nowrap ${
                  h === "Sources" || h === "In" || h === "Out" || h === "Rated USD" || h.includes("/") || h === "Snapshots"
                    ? "text-right"
                    : ""
                }`}
              >
                {h}
              </th>
            ))}
          </tr>
        </thead>
        <tbody className="divide-y divide-gray-200 text-sm">{children}</tbody>
      </table>
    </div>
  );
}
