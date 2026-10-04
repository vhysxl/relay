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
import { ExecutionStepNode } from './ExecutionStepNode'
import { formatTimestamp, formatDuration, formatJSON } from '../../utils/format'

import { mockExecutions } from '../../data/mockData'

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
    const mockMatch = mockExecutions.find((m) => m.id === executionId)

    try {
      const res = await fetch(`/executions/${encodeURIComponent(executionId)}`)
      if (!res.ok) {
        if (mockMatch) {
          setExecution(mockMatch)
          return
        }
        throw new Error(`Execution not found (HTTP ${res.status})`)
      }
      const data: Execution = await res.json()
      setExecution(data)
    } catch (err: any) {
      if (mockMatch) {
        setExecution(mockMatch)
      } else {
        setError(err.message || 'Failed to load execution')
      }
    } finally {
      setIsLoading(false)
    }
  }

  useEffect(() => {
    fetchDetail()
  }, [executionId])

  if (isLoading && !execution) {
    return (
      <div className="bg-white rounded-lg border border-slate-200/80 p-12 text-center space-y-3 shadow-xs">
        <Loader2 className="animate-spin h-8 w-8 text-emerald-600 mx-auto" />
        <p className="text-sm font-semibold text-slate-700">Loading Execution Trace...</p>
        <p className="text-xs font-mono text-slate-400">ID: {executionId}</p>
      </div>
    )
  }

  if (error || !execution) {
    return (
      <div className="bg-white rounded-lg border border-rose-200 p-8 text-center space-y-3 shadow-xs">
        <div className="w-10 h-10 rounded-full bg-rose-100 text-rose-600 mx-auto flex items-center justify-center font-bold">
          !
        </div>
        <h3 className="text-base font-bold text-slate-900">Unable to Load Execution</h3>
        <p className="text-xs text-rose-600 max-w-md mx-auto">{error}</p>
        <div className="pt-2">
          <button
            onClick={onBack}
            className="px-4 py-2 bg-slate-900 hover:bg-slate-800 text-white rounded-lg text-xs font-semibold transition cursor-pointer"
          >
            Back to Execution History
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
  const contextFormatted = formatJSON(execution.context)

  return (
    <div className="space-y-6">
      {/* Top Nav / Back button bar */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-slate-200/80">
        <div className="flex items-center space-x-3">
          <button
            onClick={onBack}
            className="inline-flex items-center space-x-1.5 px-3 py-1.5 bg-white border border-slate-200 hover:bg-slate-50 text-slate-700 text-xs font-semibold rounded-lg shadow-xs transition cursor-pointer outline-none focus:outline-none focus:ring-0"
          >
            <ArrowLeft className="w-4 h-4 text-slate-500" />
            <span>Back to History</span>
          </button>
          <div className="h-4 w-px bg-slate-200" />
          <div className="flex items-center space-x-2 text-xs font-mono text-slate-500">
            <span>Execution:</span>
            <code className="font-bold text-slate-800 bg-slate-100 px-2 py-0.5 rounded-md border border-slate-200/80">
              {execution.id}
            </code>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <button
            onClick={fetchDetail}
            disabled={isLoading}
            className="px-3.5 py-1.5 bg-white border border-slate-200 hover:bg-slate-50 text-slate-700 text-xs font-semibold rounded-lg shadow-xs flex items-center space-x-1.5 transition disabled:opacity-50 cursor-pointer outline-none focus:outline-none focus:ring-0"
          >
            <RotateCw className={`w-3.5 h-3.5 text-slate-500 ${isLoading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Summary Header Card */}
      <div className="bg-white rounded-lg border-[1.5px] border-slate-300/80 shadow-xs p-5 space-y-4">
        <div className="flex flex-col md:flex-row md:items-center md:justify-between gap-4">
          <div className="space-y-1">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="text-xl font-bold text-slate-900 tracking-tight">
                {execution.workflow_name || execution.workflow_id}
              </h2>
              <StatusBadge status={execution.status} size="md" />
            </div>
            <p className="text-xs font-mono text-slate-500">
              Workflow:{' '}
              <span className="font-semibold text-slate-700">{execution.workflow_id}</span>
            </p>
          </div>

          <div className="grid grid-cols-2 sm:grid-cols-3 gap-4 border-t md:border-t-0 md:border-l border-slate-100 pt-3 md:pt-0 md:pl-6 text-xs">
            <div>
              <p className="text-slate-400 font-medium">Started</p>
              <p className="font-mono font-semibold text-slate-700 mt-0.5">
                {formatTimestamp(execution.created_at)}
              </p>
            </div>
            <div>
              <p className="text-slate-400 font-medium">Completed</p>
              <p className="font-mono font-semibold text-slate-700 mt-0.5">
                {execution.updated_at ? formatTimestamp(execution.updated_at) : '—'}
              </p>
            </div>
            <div>
              <p className="text-slate-400 font-medium">Duration</p>
              <p className="font-mono font-semibold text-slate-700 mt-0.5">
                {formatDuration(execution.created_at, execution.updated_at)}
              </p>
            </div>
          </div>
        </div>
      </div>

      {/* Prominent Failure Alert (if failed) */}
      {(failedSteps.length > 0 ||
        execution.status === 'FAILED' ||
        execution.status === 'COMPENSATION_FAILED') && (
        <div className="bg-rose-50 border-2 border-rose-300 rounded-lg p-5 shadow-xs space-y-3">
          <div className="flex items-start space-x-3">
            <div className="w-8 h-8 rounded-md bg-rose-100 text-rose-700 flex items-center justify-center flex-shrink-0 font-bold">
              <AlertTriangle className="w-5 h-5 text-rose-600" />
            </div>
            <div className="flex-1 space-y-0.5">
              <h3 className="text-sm font-bold text-rose-900">Execution Failed</h3>
              <p className="text-xs text-rose-700">
                {failedSteps.length > 0 ? (
                  <>
                    Failure occurred at step{' '}
                    <span className="font-mono font-bold">
                      {failedSteps.map((s) => s.name || s.step_id).join(', ')}
                    </span>
                    .
                  </>
                ) : (
                  'Execution encountered an unhandled failure.'
                )}
              </p>
            </div>
          </div>

          {failedSteps.map((fs) => (
            <div
              key={fs.step_id}
              className="bg-white border border-rose-200 rounded-md p-3.5 space-y-1.5 shadow-xs"
            >
              <div className="flex items-center justify-between text-xs font-semibold text-rose-800">
                <span>
                  Step: <span className="font-mono font-bold">{fs.name || fs.step_id}</span> (
                  {fs.step_id})
                </span>
                <span className="font-mono text-slate-500">
                  Attempt {fs.attempt || 1} / {fs.max_attempts || 1}
                </span>
              </div>
              <pre className="bg-rose-50/70 border border-rose-100 rounded-md p-2.5 text-xs font-mono text-rose-900 whitespace-pre-wrap break-words leading-relaxed">
                {fs.error_message || execution.error_message || 'Step execution failed'}
              </pre>
            </div>
          ))}
        </div>
      )}

      {/* Optional Context Payload */}
      {contextFormatted && contextFormatted !== '{}' && (
        <details className="group bg-white rounded-lg border border-slate-200/80 shadow-xs overflow-hidden">
          <summary className="px-5 py-3.5 bg-slate-50/80 cursor-pointer flex items-center justify-between select-none hover:bg-slate-100/70 transition">
            <div className="flex items-center space-x-2">
              <FileCode className="w-4 h-4 text-slate-500" />
              <span className="text-xs font-bold text-slate-800 uppercase tracking-wide">
                Context Payload
              </span>
            </div>
            <ChevronDown className="w-4 h-4 text-slate-400 group-open:rotate-180 transition" />
          </summary>
          <div className="p-4 bg-slate-900 border-t border-slate-200">
            <pre className="text-emerald-400 font-mono text-xs overflow-x-auto leading-relaxed max-h-56">
              {contextFormatted}
            </pre>
          </div>
        </details>
      )}

      {/* Steps */}
      <div className="space-y-4 pt-1">
        <div className="flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <h3 className="text-base font-bold text-slate-900 tracking-tight">
              Steps
            </h3>
            <span className="text-xs font-mono font-semibold text-slate-700 bg-slate-100 px-2.5 py-0.5 rounded-md border border-slate-200/80">
              {completedStepsCount}/{steps.length}
            </span>
          </div>
        </div>

        <div>
          {steps.length === 0 ? (
            <div className="bg-white rounded-lg border border-dashed border-slate-300 p-8 text-center text-slate-400 text-xs">
              No step executions recorded for this workflow run.
            </div>
          ) : (
            steps.map((step, idx) => (
              <ExecutionStepNode
                key={step.id || step.step_id || idx}
                step={step}
                index={idx}
                isLast={idx === steps.length - 1}
              />
            ))
          )}
        </div>
      </div>
    </div>
  )
}
