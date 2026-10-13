import React, { useState, useEffect } from 'react'
import {
  ArrowLeft,
  RotateCw,
  ShieldAlert,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  Loader2,
  Workflow as WorkflowIcon,
  ListOrdered,
  FileCode,
} from 'lucide-react'
import type { Workflow, Execution } from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { formatTimestamp, formatJSON } from '../../utils/format'
import { WorkflowDagCanvas } from './WorkflowDagCanvas'

interface WorkflowDetailViewProps {
  workflowId: string
  onBack: () => void
  onNavigateToExecution?: (executionId: string) => void
}

export const WorkflowDetailView: React.FC<WorkflowDetailViewProps> = ({
  workflowId,
  onBack,
  onNavigateToExecution,
}) => {
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [recentExecutions, setRecentExecutions] = useState<Execution[]>([])
  const [isLoading, setIsLoading] = useState<boolean>(true)
  const [error, setError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'canvas' | 'list' | 'raw'>('canvas')
  const [expandedSteps, setExpandedSteps] = useState<Record<string, boolean>>({})

  const toggleStepExpand = (stepId: string) => {
    setExpandedSteps((prev) => ({ ...prev, [stepId]: !prev[stepId] }))
  }

  const fetchWorkflow = async () => {
    setIsLoading(true)
    setError(null)

    try {
      const [wfRes, execRes] = await Promise.all([
        fetch(`/workflows/${encodeURIComponent(workflowId)}`).catch(() => null),
        fetch('/executions').catch(() => null),
      ])

      if (wfRes && wfRes.ok) {
        const data = await wfRes.json()
        setWorkflow(data)
      } else {
        throw new Error(`Workflow "${workflowId}" not found`)
      }

      if (execRes && execRes.ok) {
        const execs: Execution[] = await execRes.json().catch(() => [])
        setRecentExecutions(Array.isArray(execs) ? execs.filter((e) => e.workflow_id === workflowId || e.workflow_name === workflowId || (data && e.workflow_id === data.id)) : [])
      } else {
        setRecentExecutions([])
      }
    } catch (err: any) {
      setError(err.message || 'Failed to load workflow')
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    fetchWorkflow()
  }, [workflowId])

  if (isLoading && !workflow) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-12 text-center space-y-3 shadow-xs">
        <Loader2 className="animate-spin h-7 w-7 text-neutral-400 mx-auto" />
        <p className="text-sm font-semibold text-neutral-100">Loading Workflow Definition...</p>
        <p className="text-xs font-mono text-neutral-500">{workflowId}</p>
      </div>
    )
  }

  if (error || !workflow) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-rose-900/60 p-8 text-center space-y-3 shadow-xs">
        <h3 className="text-base font-bold text-neutral-100">Workflow Not Found</h3>
        <p className="text-xs text-rose-400">{error || 'Could not find workflow'}</p>
        <div className="pt-2">
          <button
            onClick={onBack}
            className="px-3.5 py-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-100 border border-neutral-700 rounded-lg text-xs font-medium transition cursor-pointer"
          >
            Back to Workflows
          </button>
        </div>
      </div>
    )
  }

  const steps = workflow.steps || []
  const hasCompensation = steps.some((s) => s.compensation)

  return (
    <div className="space-y-4">
      {/* Top Header Block matching standard pages */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-neutral-800">
        <div className="flex items-center space-x-3">
          <button
            onClick={onBack}
            className="p-1.5 rounded-lg border border-neutral-700/80 bg-[#141414] hover:bg-neutral-800 text-neutral-400 hover:text-neutral-100 shadow-2xs transition cursor-pointer outline-none focus:outline-none focus:ring-0"
            title="Back to workflows"
          >
            <ArrowLeft className="w-4 h-4" />
          </button>

          <div className="space-y-0.5">
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold tracking-tight text-neutral-100">{workflow.name}</h1>
              <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-mono font-medium bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                v{workflow.version}
              </span>
            </div>
            <p className="text-xs font-mono text-neutral-400">ID: {workflow.id}</p>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <button
            onClick={fetchWorkflow}
            disabled={isLoading}
            className="px-3 py-1.5 bg-[#141414] border border-neutral-700/80 hover:bg-neutral-800 text-neutral-300 hover:text-neutral-100 text-xs font-medium rounded-lg shadow-2xs flex items-center space-x-1.5 transition disabled:opacity-50 cursor-pointer outline-none focus:outline-none focus:ring-0"
          >
            <RotateCw className={`w-3.5 h-3.5 text-neutral-400 ${isLoading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Summary KPI Cards */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Steps
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-2xl font-bold tracking-tight text-neutral-100">{steps.length}</span>
            <span className="text-xs text-neutral-400 font-medium">DAG Nodes</span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Rollbacks
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-2xl font-bold tracking-tight text-neutral-100">
              {hasCompensation ? 'Enabled' : 'None'}
            </span>
            <span className="text-xs text-neutral-400 font-medium">
              {steps.filter((s) => s.compensation).length} steps
            </span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Executions
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-2xl font-bold tracking-tight text-neutral-100">
              {recentExecutions.length}
            </span>
            <span className="text-xs text-neutral-400 font-medium">Runs</span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Created
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-xs font-mono font-bold text-neutral-300">
              {formatTimestamp(workflow.created_at)}
            </span>
          </div>
        </div>
      </div>

      {/* Tabs: DAG Canvas vs Step List vs Raw JSON */}
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden">
        <div className="px-5 py-3 bg-[#181818] border-b border-neutral-800 flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <button
              onClick={() => setActiveTab('canvas')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                activeTab === 'canvas'
                  ? 'bg-[#141414] text-neutral-100 shadow-2xs border border-neutral-700'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              <WorkflowIcon className="w-3.5 h-3.5" />
              <span>DAG Canvas</span>
            </button>
            <button
              onClick={() => setActiveTab('list')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                activeTab === 'list'
                  ? 'bg-[#141414] text-neutral-100 shadow-2xs border border-neutral-700'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              <ListOrdered className="w-3.5 h-3.5" />
              <span>Step List ({steps.length})</span>
            </button>
            <button
              onClick={() => setActiveTab('raw')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                activeTab === 'raw'
                  ? 'bg-[#141414] text-neutral-100 shadow-2xs border border-neutral-700'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              <FileCode className="w-3.5 h-3.5" />
              <span>Raw Definition</span>
            </button>
          </div>
        </div>

        {activeTab === 'canvas' && (
          <div className="p-3">
            <WorkflowDagCanvas steps={steps} workflowName={workflow.name} />
          </div>
        )}

        {activeTab === 'list' && (
          <div className="divide-y divide-neutral-800/80">
            {steps.length === 0 ? (
              <div className="px-5 py-10 text-center text-neutral-500 text-xs">
                No steps defined in this workflow.
              </div>
            ) : (
              steps.map((step, idx) => {
                const isExpanded = expandedSteps[step.id]
                return (
                  <div key={step.id || idx} className="p-4 space-y-3">
                    <div
                      onClick={() => toggleStepExpand(step.id)}
                      className="flex items-center justify-between cursor-pointer group"
                    >
                      <div className="flex items-center space-x-3">
                        <span className="w-6 h-6 rounded-md bg-neutral-800 text-neutral-300 font-mono font-bold text-xs flex items-center justify-center">
                          {idx + 1}
                        </span>
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="font-bold text-sm text-neutral-200 group-hover:underline">
                              {step.name}
                            </span>
                            <span className="font-mono text-xs text-neutral-500">({step.id})</span>
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center space-x-2">
                        <span className="px-2 py-0.5 rounded text-xs font-mono font-medium uppercase bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                          {step.type}
                        </span>
                        {step.compensation && (
                          <span className="px-2 py-0.5 rounded text-xs font-medium bg-amber-950/50 text-amber-400 border border-amber-800/60">
                            Rollback
                          </span>
                        )}
                        {step.retry?.max_attempts && (
                          <span className="px-2 py-0.5 rounded text-xs font-mono font-medium bg-neutral-800/80 text-neutral-400 border border-neutral-700/60">
                            {step.retry.max_attempts}x
                          </span>
                        )}
                        <span className="text-neutral-500 pl-1">
                          {isExpanded ? (
                            <ChevronDown className="w-4 h-4" />
                          ) : (
                            <ChevronRight className="w-4 h-4" />
                          )}
                        </span>
                      </div>
                    </div>

                    {/* Expanded Step Details */}
                    {isExpanded && (
                      <div className="mt-2 pl-9 space-y-3 pt-2 border-t border-neutral-800 text-xs">
                        {step.depends_on && step.depends_on.length > 0 && (
                          <div>
                            <span className="text-neutral-400 font-medium">Depends on: </span>
                            <span className="font-mono text-neutral-200">
                              {step.depends_on.join(', ')}
                            </span>
                          </div>
                        )}

                        {step.config && (
                          <div className="space-y-1">
                            <span className="text-neutral-400 font-medium">Configuration:</span>
                            <pre className="p-3 bg-[#0c0c0c] border border-neutral-800 rounded-md font-mono text-neutral-300 text-[11px] overflow-x-auto">
                              {formatJSON(step.config)}
                            </pre>
                          </div>
                        )}

                        {step.compensation && (
                          <div className="p-3 bg-neutral-900 border border-neutral-700/80 rounded-md space-y-1.5">
                            <div className="flex items-center gap-1.5 font-semibold text-amber-400">
                              <ShieldAlert className="w-3.5 h-3.5 text-amber-400" />
                              <span>Rollback: {step.compensation.name}</span>
                            </div>
                            {step.compensation.config && (
                              <pre className="p-2 bg-[#0c0c0c] border border-neutral-800 rounded font-mono text-neutral-300 text-[11px] overflow-x-auto">
                                {formatJSON(step.compensation.config)}
                              </pre>
                            )}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
        )}

        {activeTab === 'raw' && (
          <div className="p-4">
            <pre className="p-4 bg-[#0c0c0c] border border-neutral-800 text-emerald-400 font-mono text-xs rounded-lg overflow-x-auto">
              {formatJSON(workflow)}
            </pre>
          </div>
        )}
      </div>

      {/* Recent Executions Table for this Workflow */}
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden">
        <div className="px-5 py-3 bg-[#181818] border-b border-neutral-800 flex items-center justify-between">
          <h2 className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Recent Executions
          </h2>
        </div>

        <div className="divide-y divide-neutral-800/80">
          {recentExecutions.length === 0 ? (
            <div className="px-5 py-8 text-center text-neutral-500 text-xs">
              No executions recorded for this workflow yet.
            </div>
          ) : (
            recentExecutions.map((exec) => (
              <div
                key={exec.id}
                onClick={() => onNavigateToExecution && onNavigateToExecution(exec.id)}
                className="px-5 py-3 hover:bg-neutral-800/40 transition-colors group cursor-pointer flex items-center justify-between text-xs"
              >
                <div className="space-y-0.5">
                  <div className="flex items-center gap-2">
                    <span className="font-mono font-bold text-neutral-200 group-hover:underline">
                      #{exec.id.slice(0, 10)}
                    </span>
                    <span className="font-mono text-neutral-500">
                      {formatTimestamp(exec.created_at)}
                    </span>
                  </div>
                </div>
                <div className="flex items-center space-x-3">
                  <StatusBadge status={exec.status} size="sm" />
                  <ExternalLink className="w-3.5 h-3.5 text-neutral-500 group-hover:text-neutral-300 transition" />
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
