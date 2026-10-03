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
  onSelectExecution: (id: string) => void
}

export const HistoryTable: React.FC<HistoryTableProps> = ({
  executions,
  totalCount,
  isLoading = false,
  searchTerm = '',
  onSelectExecution,
}) => {
  const total = totalCount ?? executions.length

  return (
    <div className="bg-white rounded-lg border-[1.5px] border-slate-300/80 shadow-xs overflow-hidden">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-100 text-left">
          <thead className="bg-slate-50/75 border-b border-slate-200/80 text-[11px] font-semibold text-slate-500 uppercase tracking-wider">
            <tr>
              <th scope="col" className="px-5 py-3">
                Status
              </th>
              <th scope="col" className="px-5 py-3">
                Workflow ID
              </th>
              <th scope="col" className="px-5 py-3">
                Type
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
          <tbody className="divide-y divide-slate-100 bg-white text-xs">
            {isLoading && executions.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-5 py-12 text-center text-slate-400">
                  Loading history records...
                </td>
              </tr>
            ) : executions.length === 0 ? (
              <tr>
                <td colSpan={6} className="px-5 py-12 text-center text-slate-400">
                  {searchTerm
                    ? `No execution history matches "${searchTerm}".`
                    : 'No execution history found.'}
                </td>
              </tr>
            ) : (
              executions.map((item) => {
                let execType = 'http'
                if (item.steps && item.steps.length > 0) {
                  const distinct = [...new Set(item.steps.map((s) => s.type).filter(Boolean))]
                  if (distinct.length > 0) execType = distinct.join(', ')
                } else if (item.type && item.type !== 'execution') {
                  execType = item.type
                }

                const startedAt = formatTimestamp(item.created_at)
                const isRunning = item.status === 'RUNNING' || item.status === 'COMPENSATING'
                const isFinished = [
                  'COMPLETED',
                  'FAILED',
                  'COMPENSATED',
                  'COMPENSATION_FAILED',
                  'CANCELLED',
                ].includes(item.status)

                const displayName = item.workflow_name || item.name || ''

                return (
                  <tr
                    key={item.id}
                    onClick={() => onSelectExecution(item.id)}
                    className="hover:bg-slate-50/70 cursor-pointer transition-colors group"
                  >
                    <td className="px-5 py-3 whitespace-nowrap">
                      <StatusBadge status={item.status} size="sm" />
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap">
                      <span className="font-mono text-sm font-medium text-slate-900 group-hover:underline cursor-pointer">
                        {item.workflow_id}
                      </span>
                      {displayName && displayName !== item.workflow_id && (
                        <div className="text-xs text-slate-500 font-normal mt-0.5">{displayName}</div>
                      )}
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap">
                      <span className="inline-flex items-center px-2 py-0.5 rounded-md text-[11px] font-mono font-medium bg-slate-100 text-slate-700 border border-slate-200/60 uppercase">
                        {execType}
                      </span>
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap font-mono text-xs text-slate-500">
                      {startedAt}
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap font-mono text-xs text-slate-500">
                      {isFinished ? (
                        formatTimestamp(item.updated_at || item.created_at)
                      ) : isRunning ? (
                        <span className="inline-flex items-center text-xs text-slate-700 font-medium">
                          <span className="w-1.5 h-1.5 rounded-full bg-slate-700 animate-pulse mr-1.5" />
                          In Progress
                        </span>
                      ) : (
                        '—'
                      )}
                    </td>
                    <td className="px-5 py-3 whitespace-nowrap text-right">
                      <span className="inline-flex items-center space-x-1 text-xs font-medium text-slate-400 group-hover:text-slate-800 transition">
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
        <div className="px-5 py-2.5 bg-slate-50/40 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
          <span>
            Showing <strong>{executions.length}</strong> of{' '}
            <strong>{total}</strong> executions
          </span>
        </div>
      )}
    </div>
  )
}
