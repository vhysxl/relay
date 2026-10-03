import {
  Layers,
  History,
  Activity,
  ArrowRight,
  TrendingUp,
} from 'lucide-react'
import type { Workflow, Execution, ViewMode } from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { formatTimestamp } from '../../utils/format'

interface DashboardViewProps {
  workflows: Workflow[]
  executions: Execution[]
  onNavigate: (view: ViewMode, id?: string) => void
}

export const DashboardView: React.FC<DashboardViewProps> = ({
  workflows,
  executions,
  onNavigate,
}) => {
  const completedCount = executions.filter((e) => e.status === 'COMPLETED').length
  const failedCount = executions.filter(
    (e) => e.status === 'FAILED' || e.status === 'COMPENSATION_FAILED'
  ).length
  const successRate =
    executions.length > 0
      ? Math.round((completedCount / executions.length) * 100)
      : 100

  return (
    <div className="space-y-6">
      {/* Header Block */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 pb-1 border-b border-slate-200/70">
        <div>
          <h1 className="text-2xl font-bold tracking-tight text-slate-900">
            Relay Dashboard
          </h1>
          <p className="text-xs text-slate-500 font-normal mt-0.5">
            Real-time overview of DAG workflows, queue activity, and execution metrics.
          </p>
        </div>
      </div>

      {/* KPI Cards Grid */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        {/* Card 1: Workflows */}
        <div className="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-xs space-y-2">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold text-slate-500">Registered Workflows</span>
            <div className="w-8 h-8 rounded-xl bg-blue-50 text-blue-600 flex items-center justify-center">
              <Layers className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-2xl font-bold text-slate-900">{workflows.length}</span>
            <span className="text-xs text-slate-500">Definitions</span>
          </div>
        </div>

        {/* Card 2: Total Runs */}
        <div className="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-xs space-y-2">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold text-slate-500">Total Executions</span>
            <div className="w-8 h-8 rounded-xl bg-indigo-50 text-indigo-600 flex items-center justify-center">
              <History className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-2xl font-bold text-slate-900">{executions.length}</span>
            <span className="text-xs text-slate-500">Recorded Runs</span>
          </div>
        </div>

        {/* Card 3: Success Rate */}
        <div className="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-xs space-y-2">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold text-slate-500">Success Rate</span>
            <div className="w-8 h-8 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center">
              <TrendingUp className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-2xl font-bold text-emerald-600">{successRate}%</span>
            <span className="text-xs text-slate-500 font-medium">({completedCount} ok, {failedCount} failed)</span>
          </div>
        </div>

        {/* Card 4: Engine Status */}
        <div className="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-xs space-y-2">
          <div className="flex items-center justify-between text-slate-400">
            <span className="text-xs font-semibold text-slate-500">Engine Health</span>
            <div className="w-8 h-8 rounded-xl bg-emerald-50 text-emerald-600 flex items-center justify-center">
              <Activity className="w-4 h-4" />
            </div>
          </div>
          <div className="flex items-baseline space-x-2">
            <span className="text-sm font-bold text-slate-900 flex items-center gap-1.5">
              <span className="w-2.5 h-2.5 rounded-full bg-emerald-500 animate-pulse" />
              Active
            </span>
            <span className="text-xs text-slate-500">Worker Pool</span>
          </div>
        </div>
      </div>

      {/* Two Column Layout: Recent Executions & Quick Workflows */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* Left: Recent Executions */}
        <div className="bg-white rounded-2xl border border-slate-200/80 shadow-xs p-5 space-y-4">
          <div className="flex items-center justify-between border-b border-slate-100 pb-3">
            <h3 className="text-sm font-bold text-slate-900">Recent Executions</h3>
            <button
              onClick={() => onNavigate('history')}
              className="text-xs font-semibold text-blue-600 hover:text-blue-700 flex items-center gap-1 transition cursor-pointer"
            >
              <span>View All</span>
              <ArrowRight className="w-3.5 h-3.5" />
            </button>
          </div>

          <div className="space-y-2">
            {executions.slice(0, 4).map((exec) => (
              <div
                key={exec.id}
                onClick={() => onNavigate('execution-detail', exec.id)}
                className="p-3 bg-slate-50/80 hover:bg-slate-100/80 rounded-xl border border-slate-200/60 transition cursor-pointer flex items-center justify-between"
              >
                <div className="space-y-0.5">
                  <div className="flex items-center gap-2">
                    <span className="font-mono text-xs font-bold text-slate-900">
                      {exec.workflow_name || exec.workflow_id}
                    </span>
                    <span className="font-mono text-[11px] text-slate-400">
                      ({exec.id.slice(0, 10)})
                    </span>
                  </div>
                  <p className="text-[11px] font-mono text-slate-500">
                    {formatTimestamp(exec.created_at)}
                  </p>
                </div>
                <div>
                  <StatusBadge status={exec.status} size="sm" />
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Right: Registered Workflows Quick List */}
        <div className="bg-white rounded-2xl border border-slate-200/80 shadow-xs p-5 space-y-4">
          <div className="flex items-center justify-between border-b border-slate-100 pb-3">
            <h3 className="text-sm font-bold text-slate-900">Registered Workflows</h3>
            <button
              onClick={() => onNavigate('workflows')}
              className="text-xs font-semibold text-blue-600 hover:text-blue-700 flex items-center gap-1 transition cursor-pointer"
            >
              <span>View All</span>
              <ArrowRight className="w-3.5 h-3.5" />
            </button>
          </div>

          <div className="space-y-2.5">
            {workflows.map((wf) => (
              <div
                key={wf.id}
                onClick={() => onNavigate('workflows')}
                className="p-3 bg-slate-50/80 hover:bg-slate-100/80 rounded-xl border border-slate-200/60 transition cursor-pointer flex items-center justify-between"
              >
                <div className="space-y-0.5">
                  <div className="flex items-center gap-2">
                    <span className="font-bold text-xs text-slate-900">{wf.name}</span>
                    <span className="font-mono text-[10px] bg-slate-200/70 text-slate-700 px-1.5 py-0.2 rounded font-semibold">
                      v{wf.version}
                    </span>
                  </div>
                  <p className="font-mono text-[11px] text-slate-500">ID: {wf.id}</p>
                </div>
                <span className="text-xs font-mono font-semibold bg-white border border-slate-200 text-slate-700 px-2 py-0.5 rounded-lg shadow-2xs">
                  {wf.steps?.length || 0} steps
                </span>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}
