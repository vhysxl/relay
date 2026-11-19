import React, { useState, useMemo, useEffect } from 'react'
import {
  ChevronDown,
  ChevronRight,
  Copy,
  Check,
  GitBranch,
  RotateCcw,
  AlertCircle,
  Eye,
  EyeOff,
  CheckCircle2,
} from 'lucide-react'
import type { Execution, StepExecution } from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { formatTimestamp, formatJSON, formatMs } from '../../utils/format'
import { sortStepExecutionsTopologically } from '../../utils/dag'

interface ExecutionStepListProps {
  execution: Execution
}

export const ExecutionStepList: React.FC<ExecutionStepListProps> = ({ execution }) => {
  const steps = useMemo(() => execution.steps || [], [execution.steps])

  // Separate forward steps and compensation steps
  const { forwardSteps, compensationMap, orphanCompensations } = useMemo(() => {
    const forward: StepExecution[] = []
    const compMap = new Map<string, StepExecution>()

    steps.forEach((s) => {
      if (s.is_compensation) {
        compMap.set(s.step_id, s)
      } else {
        forward.push(s)
      }
    })

    const orphans: StepExecution[] = []
    steps.forEach((s) => {
      if (s.is_compensation) {
        const hasForward = forward.some((f) => f.step_id === s.step_id)
        if (!hasForward) {
          orphans.push(s)
        }
      }
    })

    const sortedForward = sortStepExecutionsTopologically(forward)

    return {
      forwardSteps: sortedForward,
      compensationMap: compMap,
      orphanCompensations: orphans,
    }
  }, [steps])

  // Check if workflow execution experienced failure, compensation, or rollback
  const hasIssues = useMemo(() => {
    return (
      execution.status === 'FAILED' ||
      execution.status === 'COMPENSATED' ||
      execution.status === 'COMPENSATION_FAILED' ||
      execution.status === 'COMPENSATING' ||
      steps.some(
        (s) =>
          s.status === 'FAILED' ||
          s.status === 'COMPENSATION_FAILED' ||
          s.status === 'COMPENSATED' ||
          s.is_compensation ||
          Boolean(s.error_message)
      )
    )
  }, [execution.status, steps])

  // By default, if the workflow experienced issues/rollbacks, hide clean successful steps so the user focuses on failures and compensations
  const [hideSuccessfulSteps, setHideSuccessfulSteps] = useState<boolean>(() => hasIssues)

  useEffect(() => {
    setHideSuccessfulSteps(hasIssues)
  }, [hasIssues, execution.id])

  // Filter steps: hide forward steps that are COMPLETED and were NOT rolled back/compensated
  const { visibleForwardSteps, hiddenSuccessfulCount } = useMemo(() => {
    if (!hideSuccessfulSteps) {
      return { visibleForwardSteps: forwardSteps, hiddenSuccessfulCount: 0 }
    }

    const visible: StepExecution[] = []
    let hiddenCount = 0

    forwardSteps.forEach((step) => {
      const compensation = compensationMap.get(step.step_id)
      const hasRollback =
        Boolean(compensation) &&
        (compensation!.status === 'COMPENSATED' ||
          compensation!.status === 'COMPENSATING' ||
          compensation!.status === 'FAILED' ||
          compensation!.status === 'COMPENSATION_FAILED')

      // Clean success means the step succeeded and was never rolled back / compensated
      const isCleanSuccess = step.status === 'COMPLETED' && !hasRollback

      if (isCleanSuccess) {
        hiddenCount++
      } else {
        visible.push(step)
      }
    })

    return { visibleForwardSteps: visible, hiddenSuccessfulCount: hiddenCount }
  }, [forwardSteps, compensationMap, hideSuccessfulSteps])

  // Expanded row state (auto-expand first failed step or first step)
  const [expandedRows, setExpandedRows] = useState<Record<string, boolean>>(() => {
    const initial: Record<string, boolean> = {}
    const failed = forwardSteps.find(
      (s) =>
        s.status === 'FAILED' ||
        s.status === 'COMPENSATION_FAILED' ||
        Boolean(s.error_message)
    )
    if (failed) {
      initial[failed.step_id] = true
    } else if (forwardSteps.length > 0) {
      initial[forwardSteps[0].step_id] = true
    }
    return initial
  })

  // Auto-expand first visible step if the currently expanded step was hidden
  useEffect(() => {
    if (visibleForwardSteps.length > 0) {
      const anyExpanded = visibleForwardSteps.some((s) => expandedRows[s.step_id])
      if (!anyExpanded) {
        const failed = visibleForwardSteps.find(
          (s) =>
            s.status === 'FAILED' ||
            s.status === 'COMPENSATION_FAILED' ||
            Boolean(s.error_message)
        )
        const target = failed || visibleForwardSteps[0]
        if (target) {
          setExpandedRows((prev) => ({ ...prev, [target.step_id]: true }))
        }
      }
    }
  }, [visibleForwardSteps])

  // Copy state per step
  const [copiedKey, setCopiedKey] = useState<string | null>(null)

  const handleCopy = (key: string, text: string) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(key)
    setTimeout(() => setCopiedKey(null), 2000)
  }

  const toggleRow = (stepId: string) => {
    setExpandedRows((prev) => ({ ...prev, [stepId]: !prev[stepId] }))
  }

  // Calculate timing & duration accurately
  const getStepTiming = (s: StepExecution) => {
    const isCancelledOrSkipped = s.status === 'CANCELLED' || s.status === 'SKIPPED' || s.status === 'WAITING'
    const hasExecuted = Boolean(
      s.executed_at ||
      (!isCancelledOrSkipped && (s.status === 'COMPLETED' || s.status === 'FAILED' || s.status === 'COMPENSATED'))
    )

    let startedAtStr = '—'
    let durationStr = '—'

    if (s.executed_at) {
      startedAtStr = formatTimestamp(s.executed_at)
      if (s.updated_at) {
        const diff = new Date(s.updated_at).getTime() - new Date(s.executed_at).getTime()
        if (diff >= 0) {
          durationStr = formatMs(diff)
        }
      }
    } else if (hasExecuted) {
      if (s.created_at) startedAtStr = formatTimestamp(s.created_at)
      if (s.created_at && s.updated_at) {
        const diff = new Date(s.updated_at).getTime() - new Date(s.created_at).getTime()
        if (diff >= 0) {
          durationStr = formatMs(diff)
        }
      }
    }

    return { startedAtStr, durationStr, hasExecuted }
  }

  if (forwardSteps.length === 0 && steps.length === 0) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-8 text-center text-neutral-500 text-xs">
        No step executions recorded.
      </div>
    )
  }

  return (
    <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden select-none">
      {/* Step List Controls Toolbar */}
      <div className="px-5 py-2.5 bg-[#161616] border-b border-neutral-800 flex items-center justify-between text-xs">
        <div className="flex items-center space-x-2">
          <span className="font-semibold text-neutral-300">Execution Steps</span>
          <span className="text-neutral-500 font-mono text-[11px]">
            ({visibleForwardSteps.length} of {forwardSteps.length} shown)
          </span>
        </div>

        {forwardSteps.length > 0 && (
          <button
            type="button"
            onClick={() => setHideSuccessfulSteps(!hideSuccessfulSteps)}
            className={`px-2.5 py-1 rounded-md border text-xs font-medium flex items-center space-x-1.5 transition cursor-pointer outline-none ${
              hideSuccessfulSteps
                ? 'bg-amber-950/40 border-amber-800/60 text-amber-300 hover:bg-amber-900/50'
                : 'bg-neutral-800 border-neutral-700/80 text-neutral-400 hover:text-neutral-200 hover:bg-neutral-700'
            }`}
            title={hideSuccessfulSteps ? 'Click to show all steps' : 'Click to hide successful steps'}
          >
            {hideSuccessfulSteps ? (
              <EyeOff className="w-3.5 h-3.5 text-amber-400" />
            ) : (
              <Eye className="w-3.5 h-3.5 text-neutral-400" />
            )}
            <span>{hideSuccessfulSteps ? 'Hide Successful' : 'Show All Steps'}</span>
            {hiddenSuccessfulCount > 0 && (
              <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-amber-900/60 text-amber-200">
                {hiddenSuccessfulCount} hidden
              </span>
            )}
          </button>
        )}
      </div>

      {/* Hidden steps notification banner */}
      {hiddenSuccessfulCount > 0 && hideSuccessfulSteps && (
        <div className="px-5 py-2 bg-neutral-900/80 border-b border-neutral-800/80 flex items-center justify-between text-xs text-neutral-400">
          <span className="flex items-center gap-1.5">
            <CheckCircle2 className="w-3.5 h-3.5 text-emerald-400 flex-shrink-0" />
            <span>
              {hiddenSuccessfulCount} successful step{hiddenSuccessfulCount > 1 ? 's' : ''} hidden — focusing on failures and rollbacks.
            </span>
          </span>
          <button
            type="button"
            onClick={() => setHideSuccessfulSteps(false)}
            className="text-neutral-300 hover:text-white underline text-[11px] cursor-pointer"
          >
            Show all steps
          </button>
        </div>
      )}

      {/* Table Header: Columns */}
      <div className="grid grid-cols-12 gap-3 items-center px-5 py-3.5 bg-[#181818] border-b border-neutral-800 text-xs sm:text-sm font-bold text-neutral-400 uppercase tracking-wider">
        <div className="col-span-5 sm:col-span-4">Step Name</div>
        <div className="col-span-2 sm:col-span-2">Status</div>
        <div className="col-span-2 sm:col-span-2">Duration</div>
        <div className="col-span-3 sm:col-span-4 flex items-center justify-between">
          <span>Started At</span>
          <span className="hidden sm:inline text-neutral-500 pr-4">Rollback</span>
        </div>
      </div>

      {/* Step Rows */}
      <div className="divide-y divide-neutral-800/80">
        {visibleForwardSteps.length === 0 && forwardSteps.length > 0 ? (
          <div className="p-8 text-center space-y-2">
            <CheckCircle2 className="w-5 h-5 text-emerald-400 mx-auto" />
            <p className="text-xs font-semibold text-neutral-200">
              All {forwardSteps.length} steps completed successfully.
            </p>
            <button
              type="button"
              onClick={() => setHideSuccessfulSteps(false)}
              className="mt-1 px-3 py-1 bg-neutral-800 hover:bg-neutral-700 text-neutral-200 rounded border border-neutral-700 text-xs transition cursor-pointer"
            >
              Show All Steps
            </button>
          </div>
        ) : (
          visibleForwardSteps.map((step, idx) => {
            const isExpanded = Boolean(expandedRows[step.step_id])
            const timing = getStepTiming(step)
            const compensation = compensationMap.get(step.step_id)
            const compTiming = compensation ? getStepTiming(compensation) : null
            const originalIndex = forwardSteps.findIndex((f) => f.step_id === step.step_id)
            const stepNumber = String((originalIndex >= 0 ? originalIndex : idx) + 1).padStart(2, '0')
            const stepError =
              step.error_message ||
              (step.status === 'FAILED' || step.status === 'COMPENSATION_FAILED'
                ? execution.error_message
                : null)

          return (
            <div key={step.id || step.step_id || idx} className="transition">
              {/* Row Header (Click to toggle expansion) */}
              <div
                onClick={() => toggleRow(step.step_id)}
                className={`grid grid-cols-12 gap-3 items-center px-5 py-3.5 cursor-pointer group transition select-none ${
                  step.status === 'FAILED' || step.status === 'COMPENSATION_FAILED'
                    ? 'bg-rose-950/20 hover:bg-rose-950/30'
                    : isExpanded
                    ? 'bg-neutral-850 hover:bg-neutral-800/80'
                    : 'hover:bg-neutral-800/40'
                }`}
              >
                {/* Column 1: Step Name & ID & Type */}
                <div className="col-span-5 sm:col-span-4 flex items-center space-x-3 truncate pr-2">
                  <span className="w-8 h-8 rounded-md bg-neutral-800 text-neutral-300 font-mono font-bold text-xs sm:text-sm flex items-center justify-center flex-shrink-0 border border-neutral-700/60 shadow-xs">
                    {stepNumber}
                  </span>
                  <div className="truncate">
                    <div className="flex items-center gap-2 truncate">
                      <span className="font-bold text-sm sm:text-base text-neutral-100 group-hover:text-white truncate">
                        {step.name || step.step_id}
                      </span>
                      <span className="px-2 py-0.5 rounded text-xs font-mono font-semibold uppercase bg-neutral-800 text-neutral-300 border border-neutral-700/60 hidden sm:inline-block">
                        {step.activity}
                      </span>
                    </div>
                    <p className="font-mono text-xs sm:text-[13px] text-neutral-400 truncate mt-0.5">
                      {step.step_id}
                    </p>
                    {stepError && (
                      <p className="font-mono text-xs sm:text-sm text-rose-400 truncate flex items-center gap-1.5 mt-1 font-medium">
                        <AlertCircle className="w-3.5 h-3.5 text-rose-400 flex-shrink-0" />
                        <span className="truncate">{stepError}</span>
                      </p>
                    )}
                  </div>
                </div>

                {/* Column 2: Status */}
                <div className="col-span-2 sm:col-span-2 flex items-center">
                  <StatusBadge status={step.status} size="md" />
                </div>

                {/* Column 3: Duration */}
                <div className="col-span-2 sm:col-span-2 font-mono text-sm sm:text-base font-semibold text-neutral-200">
                  {timing.durationStr}
                </div>

                {/* Column 4: Started At + Rollback Pill + Expand Chevron */}
                <div className="col-span-3 sm:col-span-4 flex items-center justify-between">
                  <span className="font-mono text-xs sm:text-sm text-neutral-300 truncate">
                    {timing.startedAtStr}
                  </span>

                  <div className="flex items-center space-x-2 flex-shrink-0 pl-2">
                    {compensation && (
                      <span
                        className={`px-2.5 py-1 rounded text-xs font-mono font-semibold uppercase hidden sm:inline-flex items-center gap-1.5 ${
                          compensation.status === 'COMPENSATED'
                            ? 'bg-amber-950/60 text-amber-400 border border-amber-800/60'
                            : compensation.status === 'COMPENSATING'
                            ? 'bg-amber-950/60 text-amber-300 border border-amber-800/60 animate-pulse'
                            : compensation.status === 'FAILED' || compensation.status === 'COMPENSATION_FAILED'
                            ? 'bg-rose-950/60 text-rose-400 border border-rose-800/60'
                            : 'bg-neutral-850 text-neutral-500 border border-neutral-800'
                        }`}
                        title={`Compensation: ${compensation.status}`}
                      >
                        <RotateCcw className="w-3 h-3" />
                        <span>Rollback: {compensation.status}</span>
                      </span>
                    )}

                    <span className="text-neutral-500 group-hover:text-neutral-300 transition">
                      {isExpanded ? (
                        <ChevronDown className="w-5 h-5" />
                      ) : (
                        <ChevronRight className="w-5 h-5" />
                      )}
                    </span>
                  </div>
                </div>
              </div>

              {/* Expanded Card Details */}
              {isExpanded && (
                <div className="px-5 py-4 bg-[#111111] border-t border-neutral-800 space-y-4 text-xs select-text">
                  {/* Top Metadata Badges */}
                  <div className="flex flex-wrap items-center gap-2.5 pb-3 border-b border-neutral-800 text-xs sm:text-sm font-mono text-neutral-300">
                    <span className="px-2.5 py-1 rounded bg-neutral-800 border border-neutral-700/80 text-neutral-200">
                      Step ID: <strong className="text-white">{step.step_id}</strong>
                    </span>
                    <span className="px-2.5 py-1 rounded bg-neutral-800/80 border border-neutral-700/60 text-neutral-200">
                      Attempt: {step.attempt ?? 0}/{step.max_attempts || 1}
                    </span>
                    <span className="px-2.5 py-1 rounded bg-neutral-800/80 border border-neutral-700/60 text-neutral-200">
                      Queue: {step.queue}
                    </span>
                    {step.timeout_seconds && (
                      <span className="px-2.5 py-1 rounded bg-neutral-800/80 border border-neutral-700/60 text-neutral-200">
                        Timeout: {step.timeout_seconds}s
                      </span>
                    )}
                    {step.depends_on && step.depends_on.length > 0 && (
                      <span className="px-2.5 py-1 rounded bg-neutral-800/80 border border-neutral-700/60 text-neutral-200">
                        Depends on: {step.depends_on.join(', ')}
                      </span>
                    )}
                  </div>

                  {/* Failure Alert (if this step failed) */}
                  {stepError && (
                    <div className="bg-rose-950/25 border border-rose-900/60 rounded-md p-3.5 space-y-1.5 shadow-xs">
                      <div className="flex items-center space-x-1.5 text-xs font-bold text-rose-400 uppercase tracking-wide">
                        <AlertCircle className="w-4 h-4 text-rose-400 stroke-[2]" />
                        <span>Execution Error</span>
                      </div>
                      <pre className="bg-[#0c0c0c] border border-rose-900/30 text-rose-300 p-2.5 rounded text-xs font-mono whitespace-pre-wrap break-words leading-relaxed">
                        {stepError}
                      </pre>
                    </div>
                  )}

                  {/* Condition Evaluation (if applicable) */}
                  {(step.condition_evaluation || step.when) && (
                    <div className="bg-[#141414] border border-neutral-800 rounded-lg p-3.5 space-y-2.5">
                      <div className="flex items-center justify-between pb-2 border-b border-neutral-800">
                        <div className="flex items-center space-x-2 text-xs font-bold text-neutral-200">
                          <GitBranch className="w-3.5 h-3.5 text-neutral-400" />
                          <span>Condition & Evaluation Audit</span>
                        </div>
                        {step.condition_evaluation && (
                          <span
                            className={`px-2 py-0.5 rounded text-[10px] font-mono font-bold uppercase ${
                              step.condition_evaluation.error
                                ? 'bg-rose-950/50 text-rose-400 border border-rose-800/60'
                                : step.condition_evaluation.result
                                ? 'bg-emerald-950/50 text-emerald-400 border border-emerald-800/60'
                                : 'bg-slate-900/80 text-slate-400 border border-slate-700/60'
                            }`}
                          >
                            {step.condition_evaluation.error
                              ? 'Evaluation Error'
                              : step.condition_evaluation.result
                              ? 'Condition Met (True)'
                              : 'Condition False (Skipped)'}
                          </span>
                        )}
                      </div>

                      {step.condition_evaluation?.checks && step.condition_evaluation.checks.length > 0 && (
                        <div className="overflow-x-auto rounded border border-neutral-800 bg-[#0c0c0c]">
                          <table className="w-full text-left font-mono text-xs divide-y divide-neutral-800">
                            <thead className="bg-[#181818] text-[10px] uppercase text-neutral-400 font-semibold">
                              <tr>
                                <th className="py-1.5 px-3">Anchor</th>
                                <th className="py-1.5 px-3">Op</th>
                                <th className="py-1.5 px-3">Expected</th>
                                <th className="py-1.5 px-3">Actual</th>
                                <th className="py-1.5 px-3 text-right">Result</th>
                              </tr>
                            </thead>
                            <tbody className="divide-y divide-neutral-800/80 text-[11px]">
                              {step.condition_evaluation.checks.map((c: any, i: number) => (
                                <tr key={i} className="hover:bg-neutral-800/20">
                                  <td className="py-1.5 px-3 text-neutral-300 font-semibold">{c.ref}</td>
                                  <td className="py-1.5 px-3 font-bold text-neutral-200">{c.op}</td>
                                  <td className="py-1.5 px-3 text-neutral-400">{c.expected !== undefined ? JSON.stringify(c.expected) : '—'}</td>
                                  <td className="py-1.5 px-3 text-neutral-200">{JSON.stringify(c.actual)}</td>
                                  <td className="py-1.5 px-3 text-right">
                                    {c.result ? (
                                      <span className="text-emerald-400 font-bold">pass</span>
                                    ) : (
                                      <span className="text-rose-400 font-bold">fail</span>
                                    )}
                                  </td>
                                </tr>
                              ))}
                            </tbody>
                          </table>
                        </div>
                      )}
                    </div>
                  )}

                  {/* Side-by-Side: Input Template & Output */}
                  <div className={`grid grid-cols-1 ${step.output !== undefined && step.input !== undefined ? 'lg:grid-cols-2' : ''} gap-3`}>
                    {step.input !== undefined && (
                      <div className="space-y-1.5">
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-bold text-neutral-300 uppercase tracking-wide">
                            Input Template
                          </span>
                          <button
                            onClick={() => handleCopy(`input-${step.id}`, formatJSON(step.input))}
                            className="p-1 hover:bg-neutral-800 rounded text-neutral-400 hover:text-neutral-200 transition cursor-pointer flex items-center gap-1 text-xs"
                          >
                            {copiedKey === `input-${step.id}` ? (
                              <>
                                <Check className="w-3.5 h-3.5 text-emerald-400" />
                                <span className="text-emerald-400 font-semibold">Copied</span>
                              </>
                            ) : (
                              <>
                                <Copy className="w-3.5 h-3.5" />
                                <span>Copy</span>
                              </>
                            )}
                          </button>
                        </div>
                        <pre className="bg-[#0c0c0c] border border-neutral-800 text-neutral-300 p-3 rounded font-mono text-xs sm:text-[13px] leading-relaxed overflow-x-auto max-h-56">
                          {formatJSON(step.input)}
                        </pre>
                      </div>
                    )}

                    {step.output !== undefined && (
                      <div className="space-y-1.5">
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-bold text-emerald-400 uppercase tracking-wide">
                            Output (Result)
                          </span>
                          <button
                            onClick={() => handleCopy(`output-${step.id}`, formatJSON(step.output))}
                            className="p-1 hover:bg-neutral-800 rounded text-neutral-400 hover:text-neutral-200 transition cursor-pointer flex items-center gap-1 text-xs"
                          >
                            {copiedKey === `output-${step.id}` ? (
                              <>
                                <Check className="w-3.5 h-3.5 text-emerald-400" />
                                <span className="text-emerald-400 font-semibold">Copied</span>
                              </>
                            ) : (
                              <>
                                <Copy className="w-3.5 h-3.5" />
                                <span>Copy</span>
                              </>
                            )}
                          </button>
                        </div>
                        <pre className="bg-[#0c0c0c] border border-neutral-800 text-emerald-400 p-3 rounded font-mono text-xs sm:text-[13px] leading-relaxed overflow-x-auto max-h-56">
                          {formatJSON(step.output)}
                        </pre>
                      </div>
                    )}
                  </div>

                  {/* SAGA COMPENSATION / ROLLBACK CARD (Apabila di-klik kita bisa lihat compensationnya) */}
                  {compensation ? (
                    <div className="p-4 bg-amber-950/15 border border-amber-900/40 rounded-lg space-y-3">
                      <div className="flex items-center justify-between pb-2 border-b border-amber-900/30">
                        <div className="flex items-center space-x-2">
                          <RotateCcw className="w-4 h-4 text-amber-400" />
                          <span className="font-bold text-xs sm:text-sm text-amber-300 uppercase tracking-wide">
                            Saga Compensation (Rollback Execution)
                          </span>
                        </div>
                        <StatusBadge status={compensation.status} size="md" />
                      </div>

                      {/* Compensation Metadata */}
                      <div className="grid grid-cols-2 sm:grid-cols-4 gap-2 text-xs font-mono text-neutral-400">
                        <div>
                          <span className="text-neutral-500 block text-xs">Status:</span>
                          <span className="font-semibold text-neutral-200">{compensation.status}</span>
                        </div>
                        <div>
                          <span className="text-neutral-500 block text-xs">Duration:</span>
                          <span className="font-semibold text-neutral-200">{compTiming?.durationStr}</span>
                        </div>
                        <div>
                          <span className="text-neutral-500 block text-xs">Executed At:</span>
                          <span className="font-semibold text-neutral-200">{compTiming?.startedAtStr}</span>
                        </div>
                        <div>
                          <span className="text-neutral-500 block text-xs">Attempt:</span>
                          <span className="font-semibold text-neutral-200">
                            {compensation.attempt ?? 0}/{compensation.max_attempts || 1}
                          </span>
                        </div>
                      </div>

                      {/* Compensation Error (if compensation failed) */}
                      {compensation.error_message && (
                        <div className="bg-rose-950/20 border border-rose-900/50 p-2.5 rounded text-xs font-mono text-rose-300">
                          {compensation.error_message}
                        </div>
                      )}

                      {/* Compensation Config / Payload */}
                      {compensation.input !== undefined && (
                        <div className="space-y-1">
                          <div className="flex items-center justify-between">
                            <span className="text-[10px] font-bold text-amber-300 uppercase tracking-wide">
                              Compensation Action Payload
                            </span>
                            <button
                              onClick={() => handleCopy(`comp-input-${compensation.id}`, formatJSON(compensation.input))}
                              className="p-0.5 hover:bg-neutral-800 rounded text-neutral-400 hover:text-neutral-200 transition cursor-pointer flex items-center gap-1 text-[10px]"
                            >
                              {copiedKey === `comp-input-${compensation.id}` ? (
                                <span className="text-emerald-400">Copied</span>
                              ) : (
                                <span>Copy</span>
                              )}
                            </button>
                          </div>
                          <pre className="p-2.5 bg-[#0c0c0c] border border-amber-900/30 rounded font-mono text-neutral-300 text-[11px] overflow-x-auto max-h-48">
                            {formatJSON(compensation.input)}
                          </pre>
                        </div>
                      )}

                      {/* Compensation Output (if completed) */}
                      {compensation.output !== undefined && (
                        <div className="space-y-1">
                          <span className="text-[10px] font-bold text-emerald-400 uppercase tracking-wide">
                            Compensation Result Output
                          </span>
                          <pre className="p-2.5 bg-[#0c0c0c] border border-neutral-800 text-emerald-400 rounded font-mono text-[11px] overflow-x-auto max-h-48">
                            {formatJSON(compensation.output)}
                          </pre>
                        </div>
                      )}
                    </div>
                  ) : null}
                </div>
              )}
            </div>
          )
        })
      )}

        {/* Orphan Compensations (if any standalone compensation step exists without a parent forward step) */}
        {orphanCompensations.map((step, idx) => {
          const isExpanded = Boolean(expandedRows[step.step_id || `comp-${idx}`])
          const timing = getStepTiming(step)

          return (
            <div key={step.id || idx} className="transition">
              <div
                onClick={() => toggleRow(step.step_id || `comp-${idx}`)}
                className="grid grid-cols-12 gap-3 items-center px-5 py-3.5 cursor-pointer group hover:bg-neutral-800/40 select-none"
              >
                <div className="col-span-5 sm:col-span-4 flex items-center space-x-3 truncate">
                  <span className="w-8 h-8 rounded-md bg-amber-950/60 text-amber-400 font-mono font-bold text-xs sm:text-sm flex items-center justify-center flex-shrink-0 border border-amber-800/60 shadow-xs">
                    <RotateCcw className="w-3.5 h-3.5" />
                  </span>
                  <div className="truncate">
                    <span className="font-bold text-sm sm:text-base text-amber-300 group-hover:text-amber-200 truncate">
                      [Compensate] {step.name || step.step_id}
                    </span>
                    <p className="font-mono text-xs sm:text-[13px] text-neutral-400 truncate mt-0.5">{step.step_id}</p>
                  </div>
                </div>
                <div className="col-span-2 sm:col-span-2">
                  <StatusBadge status={step.status} size="md" />
                </div>
                <div className="col-span-2 sm:col-span-2 font-mono text-sm sm:text-base font-semibold text-neutral-200">
                  {timing.durationStr}
                </div>
                <div className="col-span-3 sm:col-span-4 flex items-center justify-between">
                  <span className="font-mono text-xs sm:text-sm text-neutral-300 truncate">{timing.startedAtStr}</span>
                  <span className="text-neutral-500 group-hover:text-neutral-300">
                    {isExpanded ? <ChevronDown className="w-5 h-5" /> : <ChevronRight className="w-5 h-5" />}
                  </span>
                </div>
              </div>

              {isExpanded && (
                <div className="px-5 py-4 bg-[#111111] border-t border-neutral-800 space-y-3 text-xs">
                  {step.input !== undefined && (
                    <div>
                      <span className="text-neutral-400 font-medium">Compensation Config:</span>
                      <pre className="mt-1 p-2.5 bg-[#0c0c0c] border border-neutral-800 rounded font-mono text-neutral-300 text-[11px] overflow-x-auto">
                        {formatJSON(step.input)}
                      </pre>
                    </div>
                  )}
                </div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}
