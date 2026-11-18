import React from 'react'
import {
  Check,
  AlertCircle,
  RotateCcw,
  Loader2,
  ChevronDown,
  ArrowDown,
  SkipForward,
  GitBranch,
} from 'lucide-react'
import type { StepExecution } from '../../types/relay'
import { formatTimestamp, formatJSON } from '../../utils/format'

interface ExecutionStepNodeProps {
  step: StepExecution
  index: number
  isLast: boolean
}

export const ExecutionStepNode: React.FC<ExecutionStepNodeProps> = ({
  step,
  index,
  isLast,
}) => {
  const stepIdx = String(index + 1).padStart(2, '0')
  const isFailed =
    step.status === 'FAILED' ||
    step.status === 'COMPENSATION_FAILED' ||
    (Boolean(step.error_message) && step.error_message!.trim() !== '')

  // Step Status Badge
  let stepStatusBadge = null
  let cardAccentColor = 'border border-slate-200/80 shadow-xs'
  let leftBarColor = 'bg-slate-300'

  if (step.status === 'COMPLETED') {
    stepStatusBadge = (
      <span className="inline-flex items-center space-x-1 px-2.5 py-0.5 rounded-md text-xs font-bold bg-emerald-100 text-emerald-800 border border-emerald-300">
        <Check className="w-3.5 h-3.5 text-emerald-700 stroke-[2.5]" />
        <span>Completed</span>
      </span>
    )
    cardAccentColor = 'border border-emerald-200/90 shadow-xs'
    leftBarColor = 'bg-emerald-500'
  } else if (step.status === 'SKIPPED') {
    stepStatusBadge = (
      <span className="inline-flex items-center space-x-1 px-2.5 py-0.5 rounded-md text-xs font-bold bg-slate-100 text-slate-700 border border-slate-300">
        <SkipForward className="w-3.5 h-3.5 text-slate-500 stroke-[2.5]" />
        <span>Skipped</span>
      </span>
    )
    cardAccentColor = 'border border-slate-300 bg-slate-50/40 shadow-xs'
    leftBarColor = 'bg-slate-400'
  } else if (isFailed) {
    stepStatusBadge = (
      <span className="inline-flex items-center space-x-1 px-2.5 py-0.5 rounded-md text-xs font-bold bg-rose-100 text-rose-800 border border-rose-300">
        <AlertCircle className="w-3.5 h-3.5 text-rose-600 stroke-[2.5]" />
        <span>Failed</span>
      </span>
    )
    cardAccentColor = 'border-2 border-rose-300 bg-rose-50/10 shadow-sm'
    leftBarColor = 'bg-rose-500'
  } else if (step.status === 'RUNNING') {
    stepStatusBadge = (
      <span className="inline-flex items-center space-x-1.5 px-2.5 py-0.5 rounded-md text-xs font-bold bg-blue-100 text-blue-800 border border-blue-300">
        <Loader2 className="w-3.5 h-3.5 text-blue-600 animate-spin" />
        <span>Running</span>
      </span>
    )
    cardAccentColor = 'border-2 border-blue-300 bg-blue-50/10 shadow-sm'
    leftBarColor = 'bg-blue-500 animate-pulse'
  } else if (step.status === 'COMPENSATED') {
    stepStatusBadge = (
      <span className="inline-flex items-center space-x-1 px-2.5 py-0.5 rounded-md text-xs font-bold bg-amber-100 text-amber-800 border border-amber-300">
        <RotateCcw className="w-3.5 h-3.5 text-amber-600 stroke-[2]" />
        <span>Compensated</span>
      </span>
    )
    cardAccentColor = 'border border-amber-200/90 shadow-xs'
    leftBarColor = 'bg-amber-500'
  } else if (step.status === 'COMPENSATING') {
    stepStatusBadge = (
      <span className="inline-flex items-center space-x-1.5 px-2.5 py-0.5 rounded-md text-xs font-bold bg-amber-100 text-amber-800 border border-amber-300">
        <Loader2 className="w-3.5 h-3.5 text-amber-600 animate-spin" />
        <span>Compensating</span>
      </span>
    )
    cardAccentColor = 'border border-amber-200/90 shadow-xs'
    leftBarColor = 'bg-amber-500'
  } else {
    stepStatusBadge = (
      <span className="inline-flex items-center space-x-1 px-2.5 py-0.5 rounded-md text-xs font-medium bg-slate-100 text-slate-600 border border-slate-200">
        <span>Pending</span>
      </span>
    )
  }

  // Parse Config
  let parsedConfig: any = null
  let formattedConfig = ''
  if (step.config) {
    try {
      parsedConfig =
        typeof step.config === 'string' ? JSON.parse(step.config) : step.config
      formattedConfig = JSON.stringify(parsedConfig, null, 2)
    } catch {
      formattedConfig = String(step.config)
    }
  }

  // Parse Output
  const formattedOutput = formatJSON(step.output)

  return (
    <>
      <div
        className={`bg-white rounded-lg ${cardAccentColor} p-5 space-y-3.5 relative overflow-hidden`}
      >
        <div className={`absolute left-0 top-0 bottom-0 w-1.5 ${leftBarColor}`} />

        {/* Node Header */}
        <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2.5 border-b border-slate-100 pb-3 pl-2">
          <div className="flex items-center space-x-3">
            <span className="w-8 h-8 rounded-md bg-slate-100 text-slate-700 font-mono font-bold text-xs flex items-center justify-center border border-slate-200 shadow-xs">
              {stepIdx}
            </span>
            <div>
              <div className="flex flex-wrap items-center gap-2">
                <h4 className="text-sm font-bold text-slate-900">
                  {step.name || step.step_id}
                </h4>
                <span className="font-mono text-xs text-slate-500 bg-slate-100 px-2 py-0.5 rounded-md border border-slate-200/80">
                  ID: {step.step_id}
                </span>
              </div>
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2 pl-11 sm:pl-0">
            {stepStatusBadge}
            <span className="text-xs font-mono font-bold uppercase text-slate-800 bg-white border border-slate-300 px-2.5 py-0.5 rounded-md shadow-xs">
              {step.type}
            </span>
            {step.is_compensation && (
              <span className="text-xs font-semibold text-amber-800 bg-amber-100 border border-amber-300 px-2.5 py-0.5 rounded-md">
                Rollback
              </span>
            )}
          </div>
        </div>

        {/* Node Metadata row */}
        <div className="flex flex-wrap items-center gap-2.5 text-xs pl-2 font-mono text-slate-600">
          {step.depends_on && step.depends_on.length > 0 && (
            <span className="text-indigo-700 bg-indigo-50 border border-indigo-200 px-2.5 py-0.5 rounded-md font-medium">
              depends on: {step.depends_on.join(', ')}
            </span>
          )}
          <span className="bg-slate-100 text-slate-700 px-2.5 py-0.5 rounded-md border border-slate-200/80 font-medium">
            Attempt: {step.attempt ?? 0} / {step.max_attempts || 1}
          </span>
          {step.timeout_seconds && (
            <span className="bg-slate-100 text-slate-700 px-2.5 py-0.5 rounded-md border border-slate-200/80 font-medium">
              Timeout: {step.timeout_seconds}s
            </span>
          )}
          {step.executed_at && (
            <span className="text-slate-500">
              Executed: {formatTimestamp(step.executed_at)}
            </span>
          )}
        </div>

        {/* Condition Evaluation / When Audit */}
        {step.condition_evaluation && (
          <div className="ml-2 bg-slate-50/80 border border-slate-200/90 rounded-md p-3.5 space-y-2 text-xs">
            <div className="flex items-center justify-between">
              <div className="flex items-center space-x-1.5 font-bold text-slate-800">
                <GitBranch className="w-4 h-4 text-slate-600" />
                <span>Condition Evaluation</span>
              </div>
              <span
                className={`px-2 py-0.5 rounded text-[10px] font-mono font-bold uppercase ${
                  step.condition_evaluation.error
                    ? 'bg-rose-100 text-rose-700 border border-rose-300'
                    : step.condition_evaluation.result
                    ? 'bg-emerald-100 text-emerald-700 border border-emerald-300'
                    : 'bg-slate-200 text-slate-700 border border-slate-300'
                }`}
              >
                {step.condition_evaluation.error
                  ? 'Evaluation Error'
                  : step.condition_evaluation.result
                  ? 'Condition True'
                  : 'Condition False (Skipped)'}
              </span>
            </div>

            {step.condition_evaluation.error && (
              <p className="text-rose-600 font-mono text-[11px] bg-rose-50 p-2 rounded border border-rose-200">
                {step.condition_evaluation.error}
              </p>
            )}

            {step.condition_evaluation.checks && step.condition_evaluation.checks.length > 0 && (
              <div className="overflow-x-auto">
                <table className="w-full text-left font-mono text-[11px] divide-y divide-slate-200">
                  <thead>
                    <tr className="text-slate-500 font-semibold text-[10px] uppercase">
                      <th className="py-1 pr-2">Anchor</th>
                      <th className="py-1 px-2">Op</th>
                      <th className="py-1 px-2">Expected</th>
                      <th className="py-1 px-2">Actual</th>
                      <th className="py-1 pl-2">Result</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100">
                    {step.condition_evaluation.checks.map((c: any, i: number) => (
                      <tr key={i} className="text-slate-700">
                        <td className="py-1 pr-2 text-indigo-700 font-semibold">{c.ref}</td>
                        <td className="py-1 px-2 font-bold">{c.op}</td>
                        <td className="py-1 px-2 text-slate-600">
                          {c.expected !== undefined ? JSON.stringify(c.expected) : '—'}
                        </td>
                        <td className="py-1 px-2">
                          {c.missing ? (
                            <span className="text-amber-600 italic">[missing]</span>
                          ) : (
                            <span className="text-slate-800">{JSON.stringify(c.actual)}</span>
                          )}
                        </td>
                        <td className="py-1 pl-2">
                          {c.result ? (
                            <span className="text-emerald-600 font-bold">✓ pass</span>
                          ) : (
                            <span className="text-rose-600 font-bold">✗ fail</span>
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

        {/* Step Failure Alert */}
        {step.error_message && (
          <div className="ml-2 bg-rose-50 border border-rose-200 rounded-md p-3.5 space-y-1.5 shadow-xs">
            <div className="flex items-center space-x-1.5 text-xs font-bold text-rose-800 uppercase tracking-wide">
              <AlertCircle className="w-4 h-4 text-rose-600 stroke-[2]" />
              <span>Step Failure Error Message</span>
            </div>
            <pre className="bg-white border border-rose-200 text-rose-700 p-3 rounded-md text-xs font-mono whitespace-pre-wrap break-words leading-relaxed">
              {step.error_message}
            </pre>
          </div>
        )}

        {/* Collapsible Config & Output */}
        <div
          className={`ml-2 grid grid-cols-1 ${formattedOutput ? 'lg:grid-cols-2' : ''
            } gap-3 pt-1`}
        >
          {formattedConfig && (
            <details
              className="group bg-slate-50/70 border border-slate-200/90 rounded-md p-3 text-xs"
              open={isFailed}
            >
              <summary className="cursor-pointer font-semibold text-slate-700 flex items-center justify-between select-none hover:text-slate-900 transition">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="font-bold text-slate-800">Node Configuration</span>
                  {parsedConfig?.url && (
                    <>
                      <span className="font-mono text-xs font-bold text-slate-800 bg-slate-100 px-2 py-0.5 rounded-md border border-slate-200">
                        {parsedConfig.method || 'GET'}
                      </span>
                      <span
                        className="font-mono text-xs text-slate-600 truncate max-w-xs md:max-w-md"
                        title={parsedConfig.url}
                      >
                        {parsedConfig.url}
                      </span>
                    </>
                  )}
                  {parsedConfig?.routing_key && (
                    <span className="font-mono text-xs text-slate-600">
                      Routing Key:{' '}
                      <code className="font-bold text-slate-800">
                        {parsedConfig.routing_key}
                      </code>
                    </span>
                  )}
                </div>
                <ChevronDown className="w-3.5 h-3.5 text-slate-400 group-open:rotate-180 transition ml-2 flex-shrink-0" />
              </summary>
              <div className="mt-2.5">
                <pre className="bg-slate-900 text-slate-200 p-3 rounded-md text-xs font-mono overflow-x-auto leading-relaxed max-h-56">
                  {formattedConfig}
                </pre>
              </div>
            </details>
          )}

          {formattedOutput && (
            <details
              className="group bg-emerald-50/20 border border-emerald-200/90 rounded-md p-3 text-xs"
              open
            >
              <summary className="cursor-pointer font-semibold text-emerald-800 flex items-center justify-between select-none hover:text-emerald-900 transition">
                <span className="font-bold">Step Output / Execution Result</span>
                <ChevronDown className="w-3.5 h-3.5 text-emerald-600 group-open:rotate-180 transition ml-2 flex-shrink-0" />
              </summary>
              <div className="mt-2.5">
                <pre className="bg-slate-900 text-emerald-400 p-3 rounded-md text-xs font-mono overflow-x-auto leading-relaxed max-h-56">
                  {formattedOutput}
                </pre>
              </div>
            </details>
          )}
        </div>
      </div>

      {/* Vertical DAG Line Connector */}
      {!isLast && (
        <div className="flex justify-center py-1">
          <div className="flex flex-col items-center">
            <div className="w-0.5 h-3 bg-slate-300" />
            <div className="text-slate-400 -my-1">
              <ArrowDown className="w-4 h-4 stroke-[2.5]" />
            </div>
            <div className="w-0.5 h-3 bg-slate-300" />
          </div>
        </div>
      )}
    </>
  )
}
