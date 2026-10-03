import React from 'react'
import { ArrowRight } from 'lucide-react'
import type { Step } from '../../types/relay'

interface StepNodeBadgeProps {
  step: Step
  index: number
  isLast: boolean
}

export const StepNodeBadge: React.FC<StepNodeBadgeProps> = ({ step, index, isLast }) => {
  const stepIdx = String(index + 1).padStart(2, '0')

  return (
    <div className="flex items-center">
      <div className="flex-1 min-w-[200px] max-w-[260px] bg-slate-50/90 border border-slate-200/90 rounded-lg p-3 space-y-1.5 hover:border-slate-300 hover:bg-slate-50 transition shadow-xs">
        <div className="flex items-center justify-between">
          <span className="text-sm font-mono font-bold text-slate-500">{stepIdx}</span>
          <span className="text-xs font-mono font-bold uppercase text-slate-800 bg-white border border-slate-300 px-2 py-0.5 rounded shadow-xs">
            {step.type || 'step'}
          </span>
        </div>

        <div>
          <p className="text-sm font-bold text-slate-900 truncate" title={step.name || step.id}>
            {step.name || step.id}
          </p>
          {step.name && step.name !== step.id && (
            <p className="font-mono text-xs text-slate-500 truncate">{step.id}</p>
          )}
        </div>

        {/* Attribute badges */}
        <div className="flex flex-wrap items-center gap-1.5 pt-0.5">
          {step.depends_on && step.depends_on.length > 0 && (
            <span
              className="inline-flex items-center text-xs font-mono font-medium text-indigo-700 bg-indigo-50 border border-indigo-200 px-2 py-0.5 rounded truncate max-w-[170px]"
              title={`depends on: ${step.depends_on.join(', ')}`}
            >
              depends: {step.depends_on.join(', ')}
            </span>
          )}
          {step.compensation && (
            <span
              className="inline-flex items-center text-xs font-semibold text-amber-800 bg-amber-100/80 border border-amber-300/80 px-2 py-0.5 rounded"
              title="Has Saga rollback action"
            >
              Rollback
            </span>
          )}
          {step.retry && Boolean(step.retry.max_attempts) && (
            <span
              className="inline-flex items-center text-xs font-mono font-medium text-slate-700 bg-slate-200/70 px-2 py-0.5 rounded"
              title="Max retries"
            >
              {step.retry.max_attempts}x retry
            </span>
          )}
          {step.timeout && (
            <span className="inline-flex items-center text-xs font-mono font-medium text-slate-700 bg-slate-200/70 px-2 py-0.5 rounded">
              {step.timeout}s
            </span>
          )}
        </div>
      </div>

      {!isLast && (
        <div className="flex-shrink-0 text-slate-300 px-1.5">
          <ArrowRight className="w-4 h-4 stroke-[2.5]" />
        </div>
      )}
    </div>
  )
}
