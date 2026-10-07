import React, { useState } from 'react'
import {
  CheckCircle2,
  AlertCircle,
  RotateCcw,
  Loader2,
  Clock,
  ArrowRight,
  Copy,
  Check,
  ShieldAlert,
} from 'lucide-react'
import type { StepExecution } from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { formatTimestamp, formatJSON } from '../../utils/format'

interface ExecutionHorizontalTimelineProps {
  steps: StepExecution[]
}

export const ExecutionHorizontalTimeline: React.FC<ExecutionHorizontalTimelineProps> = ({
  steps,
}) => {
  // Auto-select first failed step, or first step
  const [selectedStepId, setSelectedStepId] = useState<string>(() => {
    if (!steps || steps.length === 0) return ''
    const failed = steps.find(
      (s) =>
        s.status === 'FAILED' ||
        s.status === 'COMPENSATION_FAILED' ||
        Boolean(s.error_message)
    )
    return failed ? failed.step_id : steps[0].step_id
  })

  const [copiedConfig, setCopiedConfig] = useState<boolean>(false)
  const [copiedOutput, setCopiedOutput] = useState<boolean>(false)

  const selectedStep =
    steps.find((s) => s.step_id === selectedStepId) || steps[0] || null

  const getStatusIcon = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return <CheckCircle2 className="w-4 h-4 text-emerald-400 flex-shrink-0" />
      case 'FAILED':
      case 'COMPENSATION_FAILED':
        return <AlertCircle className="w-4 h-4 text-rose-400 flex-shrink-0" />
      case 'RUNNING':
        return <Loader2 className="w-4 h-4 text-blue-400 animate-spin flex-shrink-0" />
      case 'COMPENSATING':
        return <Loader2 className="w-4 h-4 text-amber-400 animate-spin flex-shrink-0" />
      case 'COMPENSATED':
        return <RotateCcw className="w-4 h-4 text-amber-400 flex-shrink-0" />
      default:
        return <Clock className="w-4 h-4 text-neutral-500 flex-shrink-0" />
    }
  }

  const getStatusLeftBar = (status: string) => {
    switch (status) {
      case 'COMPLETED':
        return 'bg-emerald-500'
      case 'FAILED':
      case 'COMPENSATION_FAILED':
        return 'bg-rose-500'
      case 'RUNNING':
        return 'bg-blue-500 animate-pulse'
      case 'COMPENSATING':
      case 'COMPENSATED':
        return 'bg-amber-500'
      default:
        return 'bg-neutral-700'
    }
  }

  const handleCopyConfig = () => {
    if (!selectedStep?.config) return
    navigator.clipboard.writeText(formatJSON(selectedStep.config))
    setCopiedConfig(true)
    setTimeout(() => setCopiedConfig(false), 2000)
  }

  const handleCopyOutput = () => {
    if (!selectedStep?.output) return
    navigator.clipboard.writeText(formatJSON(selectedStep.output))
    setCopiedOutput(true)
    setTimeout(() => setCopiedOutput(false), 2000)
  }

  if (!steps || steps.length === 0) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-8 text-center text-neutral-500 text-xs">
        No step executions recorded.
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* 1. Temporal-style Horizontal Stepper Rail */}
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs">
        <div className="flex items-center space-x-2 overflow-x-auto pb-1 scrollbar-thin">
          {steps.map((step, idx) => {
            const isSelected = selectedStep?.step_id === step.step_id
            const isLast = idx === steps.length - 1
            const stepNumber = String(idx + 1).padStart(2, '0')

            return (
              <React.Fragment key={step.step_id || idx}>
                {/* Horizontal Step Card */}
                <div
                  onClick={() => setSelectedStepId(step.step_id)}
                  className={`w-64 flex-shrink-0 rounded-lg border transition-all cursor-pointer relative overflow-hidden p-3 space-y-2 select-none ${
                    isSelected
                      ? 'border-neutral-400 ring-2 ring-neutral-500/30 shadow-sm bg-[#1a1a1a]'
                      : 'border-neutral-800 hover:border-neutral-700 bg-[#121212] shadow-2xs'
                  }`}
                >
                  {/* Left Color Accent Bar */}
                  <div
                    className={`absolute left-0 top-0 bottom-0 w-1 ${getStatusLeftBar(
                      step.status
                    )}`}
                  />

                  {/* Top Row: Index + Status Icon + Type */}
                  <div className="flex items-center justify-between pl-1">
                    <div className="flex items-center space-x-1.5">
                      <span className="font-mono text-[11px] font-bold text-neutral-500">
                        {stepNumber}
                      </span>
                      {getStatusIcon(step.status)}
                      <span className="text-[11px] font-semibold text-neutral-300">
                        {step.status}
                      </span>
                    </div>

                    <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium uppercase bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                      {step.type}
                    </span>
                  </div>

                  {/* Middle Row: Name & ID */}
                  <div className="pl-1 space-y-0.5">
                    <p
                      className="font-bold text-xs text-neutral-100 truncate"
                      title={step.name || step.step_id}
                    >
                      {step.name || step.step_id}
                    </p>
                    <p
                      className="font-mono text-[10px] text-neutral-500 truncate"
                      title={step.step_id}
                    >
                      {step.step_id}
                    </p>
                  </div>

                  {/* Bottom Row: Attempt & Rollback badge */}
                  <div className="flex items-center justify-between pl-1 pt-1 border-t border-neutral-800/80 text-[10px] font-mono text-neutral-500">
                    <span>
                      Attempt {step.attempt || 1}/{step.max_attempts || 1}
                    </span>
                    {step.is_compensation && (
                      <span className="px-1.5 py-0.5 rounded text-[9px] font-medium bg-amber-950/50 text-amber-400 border border-amber-800/60 flex items-center gap-0.5">
                        <ShieldAlert className="w-2.5 h-2.5 text-amber-400" />
                        <span>Rollback</span>
                      </span>
                    )}
                  </div>
                </div>

                {/* Arrow connector between horizontal steps */}
                {!isLast && (
                  <div className="flex items-center px-1 flex-shrink-0 text-neutral-700">
                    <ArrowRight className="w-4 h-4 stroke-[2.5]" />
                  </div>
                )}
              </React.Fragment>
            )
          })}
        </div>
      </div>

      {/* 2. Active Step Inspector (Below the Horizontal Stepper) */}
      {selectedStep && (
        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs p-5 space-y-4">
          {/* Inspector Header */}
          <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-3 border-b border-neutral-800">
            <div className="space-y-0.5">
              <div className="flex items-center gap-2">
                <h4 className="text-base font-bold text-neutral-100">
                  {selectedStep.name || selectedStep.step_id}
                </h4>
                <StatusBadge status={selectedStep.status} size="sm" />
                <span className="px-2 py-0.5 rounded text-[11px] font-mono font-medium uppercase bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                  {selectedStep.type}
                </span>
              </div>
              <p className="font-mono text-xs text-neutral-400">
                ID: <span className="text-neutral-200 font-semibold">{selectedStep.step_id}</span>
              </p>
            </div>

            {/* Step Metadata Badges */}
            <div className="flex flex-wrap items-center gap-2 text-xs font-mono text-neutral-400">
              {selectedStep.depends_on && selectedStep.depends_on.length > 0 && (
                <span className="px-2 py-0.5 rounded-md bg-neutral-800 border border-neutral-700 text-neutral-300 font-medium">
                  depends: {selectedStep.depends_on.join(', ')}
                </span>
              )}
              <span className="px-2 py-0.5 rounded-md bg-neutral-800/80 border border-neutral-700/60 font-medium text-neutral-300">
                Attempt: {selectedStep.attempt || 1}/{selectedStep.max_attempts || 1}
              </span>
              {selectedStep.timeout_seconds && (
                <span className="px-2 py-0.5 rounded-md bg-neutral-800/80 border border-neutral-700/60 font-medium text-neutral-300">
                  Timeout: {selectedStep.timeout_seconds}s
                </span>
              )}
              {selectedStep.executed_at && (
                <span className="text-neutral-500">
                  Executed: {formatTimestamp(selectedStep.executed_at)}
                </span>
              )}
            </div>
          </div>

          {/* Failure Alert (if this step failed) */}
          {selectedStep.error_message && (
            <div className="bg-rose-950/20 border border-rose-900/60 rounded-md p-3.5 space-y-1.5 shadow-xs">
              <div className="flex items-center space-x-1.5 text-xs font-bold text-rose-400 uppercase tracking-wide">
                <AlertCircle className="w-4 h-4 text-rose-400 stroke-[2]" />
                <span>Error</span>
              </div>
              <pre className="bg-[#0c0c0c] border border-rose-900/30 text-rose-300 p-3 rounded-md text-xs font-mono whitespace-pre-wrap break-words leading-relaxed">
                {selectedStep.error_message}
              </pre>
            </div>
          )}

          {/* Side-by-Side: Configuration (Input) & Output (Result) */}
          <div
            className={`grid grid-cols-1 ${
              selectedStep.output && selectedStep.config ? 'lg:grid-cols-2' : ''
            } gap-4`}
          >
            {/* Configuration */}
            {selectedStep.config && (
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold text-neutral-300">Configuration</span>
                  <button
                    onClick={handleCopyConfig}
                    className="p-1 hover:bg-neutral-800 rounded text-neutral-400 hover:text-neutral-200 transition cursor-pointer flex items-center gap-1 text-[11px]"
                  >
                    {copiedConfig ? (
                      <>
                        <Check className="w-3 h-3 text-emerald-400" />
                        <span className="text-emerald-400">Copied</span>
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3" />
                        <span>Copy</span>
                      </>
                    )}
                  </button>
                </div>
                <pre className="bg-[#0c0c0c] border border-neutral-800 text-neutral-200 p-3.5 rounded-lg text-xs font-mono overflow-x-auto leading-relaxed max-h-72">
                  {formatJSON(selectedStep.config)}
                </pre>
              </div>
            )}

            {/* Output */}
            {selectedStep.output && (
              <div className="space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className="text-xs font-bold text-emerald-400">Output</span>
                  <button
                    onClick={handleCopyOutput}
                    className="p-1 hover:bg-neutral-800 rounded text-neutral-400 hover:text-neutral-200 transition cursor-pointer flex items-center gap-1 text-[11px]"
                  >
                    {copiedOutput ? (
                      <>
                        <Check className="w-3 h-3 text-emerald-400" />
                        <span className="text-emerald-400">Copied</span>
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3" />
                        <span>Copy</span>
                      </>
                    )}
                  </button>
                </div>
                <pre className="bg-[#0c0c0c] border border-neutral-800 text-emerald-400 p-3.5 rounded-lg text-xs font-mono overflow-x-auto leading-relaxed max-h-72">
                  {formatJSON(selectedStep.output)}
                </pre>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
