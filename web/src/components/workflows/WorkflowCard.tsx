import React from 'react'
import { Layers } from 'lucide-react'
import type { Workflow } from '../../types/relay'
import { StepNodeBadge } from './StepNodeBadge'
import { formatTimestamp } from '../../utils/format'

interface WorkflowCardProps {
  workflow: Workflow
}

export const WorkflowCard: React.FC<WorkflowCardProps> = ({ workflow }) => {
  const steps = workflow.steps || []

  return (
    <div className="bg-white rounded-lg border border-slate-200/80 shadow-xs p-4 space-y-3 transition-all hover:border-slate-300">
      {/* Header bar */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2.5 border-b border-slate-100 pb-2.5">
        <div className="flex items-center space-x-3">
          <div className="w-8 h-8 rounded-lg bg-slate-100 text-slate-700 border border-slate-200 flex items-center justify-center font-bold flex-shrink-0">
            <Layers className="w-4 h-4 text-emerald-600" />
          </div>
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-bold text-slate-900 text-base tracking-tight">{workflow.name}</span>
            <span className="font-mono text-xs text-slate-700 bg-slate-100 px-2 py-0.5 rounded border border-slate-200/80 font-semibold">
              v{workflow.version || 1}
            </span>
            <span className="text-xs text-slate-500 font-mono">
              ID:{' '}
              <code className="text-slate-700 bg-slate-100 px-1.5 py-0.5 rounded text-xs font-semibold">
                {workflow.id}
              </code>
            </span>
          </div>
        </div>

        <div className="flex items-center space-x-2.5 self-end sm:self-center">
          <span className="font-mono text-xs text-slate-700 bg-slate-100 px-2.5 py-1 rounded-lg border border-slate-200/80 font-semibold">
            {steps.length} Steps
          </span>
          <span className="text-xs text-slate-500 font-mono font-medium">
            {formatTimestamp(workflow.created_at)}
          </span>
        </div>
      </div>

      {/* Horizontal DAG Sequence */}
      <div>
        <div className="flex items-center overflow-x-auto pb-1.5 pt-0.5 gap-2.5">
          {steps.length === 0 ? (
            <div className="text-xs text-slate-400 py-1.5 italic">
              No steps defined for this workflow
            </div>
          ) : (
            steps.map((step, idx) => (
              <StepNodeBadge
                key={step.id || idx}
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
