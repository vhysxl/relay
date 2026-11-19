import React, { useState, useMemo } from 'react'
import { Search, RotateCw, Eye, EyeOff } from 'lucide-react'
import type { Execution } from '../../types/relay'
import { HistoryTable } from './HistoryTable'

interface HistoryViewProps {
  executions: Execution[]
  isLoading: boolean
  onRefresh: () => void
  onSelectExecution: (id: string) => void
}

type FilterTab = 'all' | 'COMPLETED' | 'RUNNING' | 'FAILED' | 'COMPENSATED'

export const HistoryView: React.FC<HistoryViewProps> = ({
  executions,
  isLoading,
  onRefresh,
  onSelectExecution,
}) => {
  const [searchTerm, setSearchTerm] = useState('')
  const [currentFilter, setCurrentFilter] = useState<FilterTab>('all')
  const [hideSuccessful, setHideSuccessful] = useState<boolean>(true)

  const completedCount = useMemo(() => {
    return executions.filter((i) => i.status === 'COMPLETED').length
  }, [executions])

  const filteredExecutions = useMemo(() => {
    let list = executions

    if (currentFilter === 'COMPLETED') {
      list = list.filter((i) => i.status === 'COMPLETED')
    } else {
      if (hideSuccessful) {
        list = list.filter((i) => i.status !== 'COMPLETED')
      }

      if (currentFilter === 'RUNNING') {
        list = list.filter((i) => i.status === 'RUNNING' || i.status === 'COMPENSATING')
      } else if (currentFilter === 'FAILED') {
        list = list.filter((i) => i.status === 'FAILED' || i.status === 'COMPENSATION_FAILED')
      } else if (currentFilter === 'COMPENSATED') {
        list = list.filter((i) => i.status === 'COMPENSATED')
      }
    }

    const q = searchTerm.trim().toLowerCase()
    if (q) {
      list = list.filter(
        (i) =>
          (i.workflow_id && i.workflow_id.toLowerCase().includes(q)) ||
          (i.workflow_name && i.workflow_name.toLowerCase().includes(q)) ||
          (i.id && i.id.toLowerCase().includes(q))
      )
    }

    return list
  }, [executions, currentFilter, searchTerm, hideSuccessful])

  return (
    <div className="space-y-4">
      {/* Page Header Block matching Workflows page */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-neutral-800">
        <div className="space-y-0.5">
          <h1 className="text-xl font-bold tracking-tight text-neutral-100">Execution History</h1>
          <p className="text-xs text-neutral-400 font-normal">
            Audit log of workflow executions
          </p>
        </div>

        <div className="flex items-center space-x-2">
          <div className="relative">
            <input
              type="text"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder="Search executions..."
              className="pl-8 pr-3 py-1.5 text-xs bg-[#141414] border border-neutral-700/80 rounded-lg focus:outline-none focus:ring-1 focus:ring-neutral-500 focus:border-neutral-500 w-56 text-neutral-200 placeholder-neutral-500 shadow-2xs"
            />
            <Search className="w-3.5 h-3.5 text-neutral-500 absolute left-2.5 top-2.5" />
          </div>

          <button
            onClick={onRefresh}
            disabled={isLoading}
            className="px-3 py-1.5 bg-[#141414] border border-neutral-700/80 hover:bg-neutral-800 text-neutral-300 hover:text-neutral-100 text-xs font-medium rounded-lg shadow-2xs flex items-center space-x-1.5 transition disabled:opacity-50 cursor-pointer outline-none focus:outline-none focus:ring-0"
          >
            <RotateCw className={`w-3.5 h-3.5 text-neutral-400 ${isLoading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Filter Tabs & Hide Successful Toggle */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
        <div className="inline-flex p-0.5 bg-neutral-900 border border-neutral-800 rounded-lg text-xs font-medium text-neutral-400">
          <button
            onClick={() => setCurrentFilter('all')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'all'
                ? 'bg-[#181818] text-neutral-100 shadow-xs font-medium border border-neutral-700/60'
                : 'hover:text-neutral-200'
            }`}
          >
            All
          </button>
          <button
            onClick={() => {
              setCurrentFilter('COMPLETED')
              setHideSuccessful(false)
            }}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'COMPLETED'
                ? 'bg-[#181818] text-neutral-100 shadow-xs font-medium border border-neutral-700/60'
                : 'hover:text-neutral-200'
            }`}
          >
            Completed
          </button>
          <button
            onClick={() => setCurrentFilter('RUNNING')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'RUNNING'
                ? 'bg-[#181818] text-neutral-100 shadow-xs font-medium border border-neutral-700/60'
                : 'hover:text-neutral-200'
            }`}
          >
            Running
          </button>
          <button
            onClick={() => setCurrentFilter('FAILED')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'FAILED'
                ? 'bg-[#181818] text-rose-400 shadow-xs font-medium border border-rose-900/60'
                : 'text-rose-400/90 hover:text-rose-300'
            }`}
          >
            Failed
          </button>
          <button
            onClick={() => setCurrentFilter('COMPENSATED')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'COMPENSATED'
                ? 'bg-[#181818] text-amber-400 shadow-xs font-medium border border-amber-900/60'
                : 'text-amber-400/90 hover:text-amber-300'
            }`}
          >
            Compensated
          </button>
        </div>

        {/* Toggle: Hide Successful */}
        <button
          type="button"
          onClick={() => {
            const next = !hideSuccessful
            setHideSuccessful(next)
            if (next && currentFilter === 'COMPLETED') {
              setCurrentFilter('all')
            }
          }}
          className={`px-3 py-1.5 rounded-lg border text-xs font-medium flex items-center space-x-2 transition cursor-pointer outline-none ${
            hideSuccessful
              ? 'bg-amber-950/40 border-amber-800/60 text-amber-300 hover:bg-amber-900/50'
              : 'bg-[#141414] border-neutral-700/80 text-neutral-400 hover:text-neutral-200 hover:bg-neutral-800'
          }`}
          title={
            hideSuccessful
              ? 'Successful executions are hidden. Click to show.'
              : 'Click to hide successful executions.'
          }
        >
          {hideSuccessful ? (
            <EyeOff className="w-3.5 h-3.5 text-amber-400" />
          ) : (
            <Eye className="w-3.5 h-3.5 text-neutral-400" />
          )}
          <span>Hide Successful</span>
          {completedCount > 0 && (
            <span
              className={`px-1.5 py-0.5 rounded text-[10px] font-mono ${
                hideSuccessful
                  ? 'bg-amber-900/60 text-amber-200'
                  : 'bg-neutral-800 text-neutral-400'
              }`}
            >
              {completedCount}
            </span>
          )}
        </button>
      </div>

      {/* History Table */}
      <HistoryTable
        executions={filteredExecutions}
        totalCount={executions.length}
        hiddenCompletedCount={hideSuccessful ? completedCount : 0}
        onShowCompleted={() => setHideSuccessful(false)}
        isLoading={isLoading}
        searchTerm={searchTerm}
        onSelectExecution={onSelectExecution}
      />
    </div>
  )
}
