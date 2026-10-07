import React from 'react'
import { Check, AlertCircle, RotateCcw, Loader2 } from 'lucide-react'
import type { ExecutionStatus } from '../../types/relay'

interface StatusBadgeProps {
  status: ExecutionStatus
  size?: 'sm' | 'md'
}

export const StatusBadge: React.FC<StatusBadgeProps> = ({ status, size = 'sm' }) => {
  const isMd = size === 'md'
  const iconClass = isMd ? 'w-3.5 h-3.5' : 'w-3 h-3'

  switch (status) {
    case 'COMPLETED':
      return (
        <span
          className={`inline-flex items-center gap-1.5 font-bold rounded-lg border bg-emerald-950/40 text-emerald-400 border-emerald-800/60 ${
            isMd ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-xs'
          }`}
        >
          <Check className={`${iconClass} text-emerald-400 stroke-[2.5]`} />
          <span>COMPLETED</span>
        </span>
      )

    case 'RUNNING':
      return (
        <span
          className={`inline-flex items-center gap-1.5 font-bold rounded-lg border bg-blue-950/40 text-blue-400 border-blue-800/60 ${
            isMd ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-xs'
          }`}
        >
          <Loader2 className={`${iconClass} text-blue-400 animate-spin`} />
          <span>RUNNING</span>
        </span>
      )

    case 'FAILED':
      return (
        <span
          className={`inline-flex items-center gap-1.5 font-bold rounded-lg border bg-rose-950/40 text-rose-400 border-rose-800/60 ${
            isMd ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-xs'
          }`}
        >
          <AlertCircle className={`${iconClass} text-rose-400 stroke-[2.5]`} />
          <span>FAILED</span>
        </span>
      )

    case 'COMPENSATING':
      return (
        <span
          className={`inline-flex items-center gap-1.5 font-bold rounded-lg border bg-amber-950/40 text-amber-400 border-amber-800/60 ${
            isMd ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-xs'
          }`}
        >
          <Loader2 className={`${iconClass} text-amber-400 animate-spin`} />
          <span>COMPENSATING</span>
        </span>
      )

    case 'COMPENSATED':
      return (
        <span
          className={`inline-flex items-center gap-1.5 font-bold rounded-lg border bg-amber-950/40 text-amber-400 border-amber-800/60 ${
            isMd ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-xs'
          }`}
        >
          <RotateCcw className={`${iconClass} text-amber-400 stroke-[2]`} />
          <span>COMPENSATED</span>
        </span>
      )

    case 'COMPENSATION_FAILED':
      return (
        <span
          className={`inline-flex items-center gap-1.5 font-bold rounded-lg border bg-rose-950/40 text-rose-400 border-rose-800/60 ${
            isMd ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-xs'
          }`}
        >
          <AlertCircle className={`${iconClass} text-rose-400 stroke-[2.5]`} />
          <span>COMPENSATION FAILED</span>
        </span>
      )

    default:
      return (
        <span
          className={`inline-flex items-center gap-1 font-semibold rounded-lg border bg-neutral-900 text-neutral-400 border-neutral-800 uppercase ${
            isMd ? 'px-3 py-1 text-xs' : 'px-2.5 py-0.5 text-xs'
          }`}
        >
          <span>{status || 'QUEUED'}</span>
        </span>
      )
  }
}
