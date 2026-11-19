import React from 'react'
import { ChevronRight } from 'lucide-react'
import type { Execution } from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { formatTimestamp } from '../../utils/format'

interface HistoryTableProps {
  executions: Execution[]
  totalCount?: number
  isLoading?: boolean
  searchTerm?: string
  hiddenCompletedCount?: number
  onShowCompleted?: () => void
  onSelectExecution: (id: string) => void
}

export const HistoryTable: React.FC<HistoryTableProps> = ({
  executions,
  totalCount,
  isLoading = false,
  searchTerm = '',
  hiddenCompletedCount = 0,
  onShowCompleted,
  onSelectExecution,
}) => {
  const total = totalCount ?? executions.length

  return (
    <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-neutral-800/80 text-left">
          <thead className="bg-[#181818] border-b border-neutral-800 text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            <tr>
              <th scope="col" className="px-5 py-3">
                Status
              </th>
              <th scope="col" className="px-5 py-3">
                Workflow
              </th>
              <th scope="col" className="px-5 py-3">
                Activity
              </th>
              <th scope="col" className="px-5 py-3">
                Started At
              </th>
              <th scope="col" className="px-5 py-3">
                End At
              </th>
              <th scope="col" className="px-5 py-3 text-right">
                <span className="sr-only">Details</span>
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-neutral-800/80 bg-[#141414] text-xs">
            {isLoading && executions.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-5 py-12 text-center text-neutral-500">
                  Loading history records...
                </td>
              </tr>
            ) : executions.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-5 py-12 text-center text-neutral-500">
                  {searchTerm ? (
                    `No execution history matches "${searchTerm}".`
                  ) : hiddenCompletedCount > 0 ? (
                    <div className="space-y-2">
                      <p className="text-xs font-semibold text-neutral-300">
                        No failed or compensated executions found.
                      </p>
                      <p className="text-[11px] text-neutral-500">
                        {hiddenCompletedCount} successful execution{hiddenCompletedCount > 1 ? 's are' : ' is'} hidden.
                      </p>
                      {onShowCompleted && (
                        <button
                          type="button"
                          onClick={onShowCompleted}
                          className="mt-1 px-3 py-1 bg-neutral-800 hover:bg-neutral-700 text-neutral-200 rounded border border-neutral-700 text-xs transition cursor-pointer"
                        >
                          Show All Executions
                        </button>
                      )}
                    </div>
                  ) : (
                    'No execution history found.'
                  )}
                </td>
              </tr>
            ) : (
              executions.map((item) => {
                let activities = '—'
                if (item.steps && item.steps.length > 0) {
                  const distinct = [...new Set(item.steps.map((s) => s.activity).filter(Boolean))]
                  if (distinct.length > 0) activities = distinct.join(', ')
                }

                const startedAt = formatTimestamp(item.created_at)
                const isFinished = [
                  'COMPLETED',
                  'FAILED',
                  'COMPENSATED',
                  'COMPENSATION_FAILED',
                  'CANCELLED',
                ].includes(item.status)
                const isFailed = item.status === 'FAILED' || item.status === 'COMPENSATION_FAILED'

                const displayName = item.workflow_name || item.workflow_id

                return (
                  <tr
                    key={item.id}
                    onClick={() => onSelectExecution(item.id)}
                    className={`cursor-pointer transition-colors group ${
                      isFailed
                        ? 'bg-rose-950/20 hover:bg-rose-950/30'
                        : 'hover:bg-neutral-800/40'
                    }`}
                  >
                    <td className="px-5 py-3 whitespace-nowrap">
                      <StatusBadge status={item.status} size="sm" />
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap">
                      <span className="font-mono text-sm font-medium text-neutral-200 group-hover:underline cursor-pointer">
                        {item.workflow_id}
                      </span>
                      {displayName && displayName !== item.workflow_id && (
                        <div className="text-xs text-neutral-500 font-normal mt-0.5">{displayName}</div>
                      )}
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap">
                      <span className="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-mono font-medium bg-neutral-800/80 text-neutral-300 border border-neutral-700/60 uppercase">
                        {activities}
                      </span>
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap font-mono text-sm text-neutral-400">
                      {startedAt}
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap font-mono text-sm text-neutral-400">
                      {isFinished ? formatTimestamp(item.updated_at || item.created_at) : '—'}
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap text-right">
                      <span className="inline-flex items-center space-x-1 text-xs font-medium text-neutral-500 group-hover:text-neutral-300 transition">
                        <span>Details</span>
                        <ChevronRight className="w-3.5 h-3.5 transform group-hover:translate-x-0.5 transition" />
                      </span>
                    </td>
                  </tr>
                )
              })
            )}
          </tbody>
        </table>
      </div>

      {/* Footer info matching WorkflowsView */}
      {executions.length > 0 && (
        <div className="px-5 py-2.5 bg-[#181818] border-t border-neutral-800 flex items-center justify-between text-xs text-neutral-400">
          <span>
            Showing <strong>{executions.length}</strong> of{' '}
            <strong>{total}</strong> executions
          </span>
        </div>
      )}
    </div>
  )
}
