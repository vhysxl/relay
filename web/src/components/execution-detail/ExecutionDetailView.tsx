import React, { useState, useEffect } from 'react'
import {
  ArrowLeft,
  RotateCw,
  AlertTriangle,
  FileCode,
  ChevronDown,
  Loader2,
} from 'lucide-react'
import type { Execution } from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { ExecutionHorizontalTimeline } from './ExecutionHorizontalTimeline'
import { formatTimestamp, formatDuration, formatJSON } from '../../utils/format'

interface ExecutionDetailViewProps {
  executionId: string
  onBack: () => void
}

export const ExecutionDetailView: React.FC<ExecutionDetailViewProps> = ({
  executionId,
  onBack,
}) => {
  const [execution, setExecution] = useState<Execution | null>(null)
  const [isLoading, setIsLoading] = useState<boolean>(true)
  const [error, setError] = useState<string | null>(null)

  const fetchDetail = async () => {
    setIsLoading(true)
    setError(null)

    try {
      const res = await fetch(`/executions/${encodeURIComponent(executionId)}`)
      if (!res.ok) {
        throw new Error(`Execution "${executionId}" not found (HTTP ${res.status})`)
      }
      const data: Execution = await res.json()
      setExecution(data)
    } catch (err: any) {
      setError(err.message || 'Failed to load execution')
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    fetchDetail()
  }, [executionId])

  if (isLoading && !execution) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-12 text-center space-y-3 shadow-xs">
        <Loader2 className="animate-spin h-7 w-7 text-neutral-400 mx-auto" />
        <p className="text-sm font-semibold text-neutral-100">Loading Execution...</p>
        <p className="text-xs font-mono text-neutral-500">ID: {executionId}</p>
      </div>
    )
  }

  if (error || !execution) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-rose-900/60 p-8 text-center space-y-3 shadow-xs">
        <h3 className="text-base font-bold text-neutral-100">Execution Not Found</h3>
        <p className="text-xs text-rose-400">{error || 'Could not find execution'}</p>
        <div className="pt-2">
          <button
            onClick={onBack}
            className="px-3.5 py-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-100 border border-neutral-700 rounded-lg text-xs font-medium transition cursor-pointer"
          >
            Back to History
          </button>
        </div>
      </div>
    )
  }

  const steps = execution.steps || []
  const failedSteps = steps.filter(
    (s) =>
      s.status === 'FAILED' ||
      s.status === 'COMPENSATION_FAILED' ||
      (Boolean(s.error_message) && s.error_message!.trim() !== '')
  )
  const completedStepsCount = steps.filter((s) => s.status === 'COMPLETED').length
  const skippedStepsCount = steps.filter((s) => s.status === 'SKIPPED').length
  const contextFormatted = formatJSON(execution.context)

  return (
    <div className="space-y-4">
      {/* Top Nav / Back Button Bar */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-neutral-800">
        <div className="flex items-center space-x-3">
          <button
            onClick={onBack}
            className="p-1.5 rounded-lg border border-neutral-700/80 bg-[#141414] hover:bg-neutral-800 text-neutral-400 hover:text-neutral-100 shadow-2xs transition cursor-pointer outline-none focus:outline-none focus:ring-0"
            title="Back to history"
          >
            <ArrowLeft className="w-4 h-4" />
          </button>

          <div className="space-y-0.5">
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold tracking-tight text-neutral-100">
                {execution.workflow_name || execution.workflow_id}
              </h1>
              <StatusBadge status={execution.status} size="sm" />
            </div>
            <p className="text-xs font-mono text-neutral-400">
              Execution: <span className="font-semibold text-neutral-200">{execution.id}</span>
              <span className="mx-2 text-neutral-700">|</span>
              Workflow: <span className="text-neutral-200">{execution.workflow_id}</span>
            </p>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <button
            onClick={fetchDetail}
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
            Status
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-xl font-bold tracking-tight text-neutral-100">
              {execution.status}
            </span>
            <span className="text-xs font-mono text-neutral-400">
              {completedStepsCount} ok{skippedStepsCount > 0 ? `, ${skippedStepsCount} skip` : ''} / {steps.length}
            </span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Started
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-xs font-mono font-bold text-neutral-300">
              {formatTimestamp(execution.created_at)}
            </span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Completed
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-xs font-mono font-bold text-neutral-300">
              {execution.updated_at ? formatTimestamp(execution.updated_at) : '—'}
            </span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Duration
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-xl font-mono font-bold text-neutral-100">
              {formatDuration(execution.created_at, execution.updated_at)}
            </span>
          </div>
        </div>
      </div>

      {/* Prominent Failure Alert (if failed) */}
      {(failedSteps.length > 0 ||
        execution.status === 'FAILED' ||
        execution.status === 'COMPENSATION_FAILED') && (
        <div className="bg-rose-950/20 border border-rose-900/60 rounded-lg p-4 shadow-xs space-y-2">
          <div className="flex items-center space-x-2 text-rose-400 font-bold text-xs uppercase tracking-wide">
            <AlertTriangle className="w-4 h-4 text-rose-400" />
            <span>Execution Failed</span>
          </div>
          {failedSteps.map((fs) => (
            <div
              key={fs.step_id}
              className="bg-[#141414] border border-rose-900/40 rounded-md p-3 space-y-1 text-xs"
            >
              <div className="flex items-center justify-between font-semibold text-rose-300">
                <span>
                  Step: <strong className="font-mono">{fs.name || fs.step_id}</strong> ({fs.step_id})
                </span>
                <span className="font-mono text-neutral-400">
                  Attempt {fs.attempt ?? 0}/{fs.max_attempts || 1}
                </span>
              </div>
              <pre className="bg-[#0c0c0c] border border-rose-900/30 rounded p-2 text-xs font-mono text-rose-300 whitespace-pre-wrap break-words">
                {fs.error_message || execution.error_message || 'Step execution failed'}
              </pre>
            </div>
          ))}
        </div>
      )}

      {/* Collapsible Context Payload */}
      {contextFormatted && contextFormatted !== '{}' && (
        <details className="group bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden">
          <summary className="px-5 py-3 bg-[#181818] cursor-pointer flex items-center justify-between select-none hover:bg-neutral-800/40 transition">
            <div className="flex items-center space-x-2">
              <FileCode className="w-4 h-4 text-neutral-400" />
              <span className="text-xs font-bold text-neutral-300 uppercase tracking-wider">
                Context Payload
              </span>
            </div>
            <ChevronDown className="w-4 h-4 text-neutral-500 group-open:rotate-180 transition" />
          </summary>
          <div className="p-4 bg-[#0c0c0c] border-t border-neutral-800">
            <pre className="text-emerald-400 font-mono text-xs overflow-x-auto leading-relaxed max-h-56">
              {contextFormatted}
            </pre>
          </div>
        </details>
      )}

      {/* Horizontal Steps Stepper + Active Inspector */}
      <div className="space-y-2">
        <div className="flex items-center space-x-2 pt-1">
          <h3 className="text-sm font-bold text-neutral-100">Steps</h3>
          <span className="text-xs font-mono font-medium text-neutral-300 bg-neutral-800/80 px-2 py-0.5 rounded border border-neutral-700/60">
            {completedStepsCount} ok{skippedStepsCount > 0 ? `, ${skippedStepsCount} skipped` : ''} / {steps.length}
          </span>
        </div>

        <ExecutionHorizontalTimeline steps={steps} />
      </div>
    </div>
  )
}
