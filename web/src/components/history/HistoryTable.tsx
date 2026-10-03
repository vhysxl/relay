import React from 'react'
import { ChevronRight } from 'lucide-react'
import type { Execution } from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { formatTimestamp } from '../../utils/format'

interface HistoryTableProps {
  executions: Execution[]
  onSelectExecution: (id: string) => void
}

export const HistoryTable: React.FC<HistoryTableProps> = ({
  executions,
  onSelectExecution,
}) => {
  if (executions.length === 0) {
    return (
      <div className="bg-white rounded-lg border-2 border-slate-200 shadow-xs overflow-hidden">
        <div className="px-6 py-12 text-center text-slate-400 text-xs">
          No execution history matches the selected filter.
        </div>
      </div>
    )
  }

  return (
    <div className="bg-white rounded-lg border-2 border-slate-200 shadow-xs overflow-hidden">
      <div className="overflow-x-auto">
        <table className="min-w-full divide-y divide-slate-200 text-left">
          <thead className="bg-slate-50/80 border-b-2 border-slate-200 text-[11px] font-semibold text-slate-500 uppercase tracking-wider">
            <tr>
              <th scope="col" className="px-6 py-3.5">
                Status
              </th>
              <th scope="col" className="px-6 py-3.5">
                Workflow ID
              </th>
              <th scope="col" className="px-6 py-3.5">
                Type
              </th>
              <th scope="col" className="px-6 py-3.5">
                Started At
              </th>
              <th scope="col" className="px-6 py-3.5">
                End At
              </th>
              <th scope="col" className="px-6 py-3.5 text-right">
                <span className="sr-only">Details</span>
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-200 bg-white text-xs">
            {executions.map((item) => {
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
                  className="hover:bg-slate-50 cursor-pointer transition group"
                >
                  <td className="px-6 py-4 whitespace-nowrap">
                    <StatusBadge status={item.status} size="sm" />
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <div className="font-mono text-sm font-bold text-slate-900 group-hover:text-emerald-700 transition">
                      {item.workflow_id}
                    </div>
                    {displayName && (
                      <div className="text-xs text-slate-500 font-medium">{displayName}</div>
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap">
                    <span className="inline-flex items-center px-2.5 py-0.5 rounded-md font-mono text-xs font-semibold bg-slate-100 text-slate-700 border border-slate-200 uppercase">
                      {execType}
                    </span>
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap font-mono text-xs text-slate-600">
                    {startedAt}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap font-mono text-xs text-slate-600">
                    {isFinished ? (
                      formatTimestamp(item.updated_at || item.created_at)
                    ) : isRunning ? (
                      <span className="inline-flex items-center text-xs text-blue-600 font-medium">
                        <span className="w-1.5 h-1.5 rounded-full bg-blue-500 animate-pulse mr-1.5" />
                        In Progress
                      </span>
                    ) : (
                      '—'
                    )}
                  </td>
                  <td className="px-6 py-4 whitespace-nowrap text-right">
                    <span className="inline-flex items-center space-x-1 text-xs font-semibold text-slate-400 group-hover:text-emerald-600 transition">
                      <span>Details</span>
                      <ChevronRight className="w-4 h-4 transform group-hover:translate-x-0.5 transition" />
                    </span>
                  </td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
