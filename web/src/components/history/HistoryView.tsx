import React, { useState, useMemo } from 'react'
import { Search, RotateCw } from 'lucide-react'
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

  const filteredExecutions = useMemo(() => {
    let list = executions

    if (currentFilter === 'COMPLETED') {
      list = list.filter((i) => i.status === 'COMPLETED')
    } else if (currentFilter === 'RUNNING') {
      list = list.filter((i) => i.status === 'RUNNING' || i.status === 'COMPENSATING')
    } else if (currentFilter === 'FAILED') {
      list = list.filter((i) => i.status === 'FAILED' || i.status === 'COMPENSATION_FAILED')
    } else if (currentFilter === 'COMPENSATED') {
      list = list.filter((i) => i.status === 'COMPENSATED')
    }

    const q = searchTerm.trim().toLowerCase()
    if (q) {
      list = list.filter(
        (i) =>
          (i.workflow_id && i.workflow_id.toLowerCase().includes(q)) ||
          (i.workflow_name && i.workflow_name.toLowerCase().includes(q)) ||
          (i.name && i.name.toLowerCase().includes(q)) ||
          (i.id && i.id.toLowerCase().includes(q))
      )
    }

    return list
  }, [executions, currentFilter, searchTerm])

  return (
    <div className="space-y-4">
      {/* Page Header Block matching Workflows page */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-slate-200/70">
        <div className="space-y-0.5">
          <h1 className="text-xl font-bold tracking-tight text-slate-900">Execution History</h1>
          <p className="text-xs text-slate-500 font-normal">
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
              className="pl-8 pr-3 py-1.5 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-1 focus:ring-slate-400 focus:border-slate-400 w-56 text-slate-700 placeholder-slate-400 shadow-2xs"
            />
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-2.5 top-2.5" />
          </div>

          <button
            onClick={onRefresh}
            disabled={isLoading}
            className="px-3 py-1.5 bg-white border border-slate-200 hover:bg-slate-50 text-slate-700 text-xs font-medium rounded-lg shadow-2xs flex items-center space-x-1.5 transition disabled:opacity-50 cursor-pointer outline-none focus:outline-none focus:ring-0"
          >
            <RotateCw className={`w-3.5 h-3.5 text-slate-500 ${isLoading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Filter Tabs for History */}
      <div className="flex items-center justify-between">
        <div className="inline-flex p-0.5 bg-slate-200/60 rounded-lg text-xs font-medium text-slate-600">
          <button
            onClick={() => setCurrentFilter('all')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'all'
                ? 'bg-white text-slate-900 shadow-xs font-medium'
                : 'hover:text-slate-900'
            }`}
          >
            All
          </button>
          <button
            onClick={() => setCurrentFilter('COMPLETED')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'COMPLETED'
                ? 'bg-white text-slate-900 shadow-xs font-medium'
                : 'hover:text-slate-900'
            }`}
          >
            Completed
          </button>
          <button
            onClick={() => setCurrentFilter('RUNNING')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'RUNNING'
                ? 'bg-white text-slate-900 shadow-xs font-medium'
                : 'hover:text-slate-900'
            }`}
          >
            Running
          </button>
          <button
            onClick={() => setCurrentFilter('FAILED')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'FAILED'
                ? 'bg-white text-rose-700 shadow-xs font-medium'
                : 'text-rose-600 hover:text-rose-700'
            }`}
          >
            Failed
          </button>
          <button
            onClick={() => setCurrentFilter('COMPENSATED')}
            className={`px-3 py-1 rounded-md transition cursor-pointer outline-none focus:outline-none focus:ring-0 ${
              currentFilter === 'COMPENSATED'
                ? 'bg-white text-amber-800 shadow-xs font-medium'
                : 'text-amber-600 hover:text-amber-700'
            }`}
          >
            Compensated
          </button>
        </div>
      </div>

      {/* History Table */}
      <HistoryTable
        executions={filteredExecutions}
        totalCount={executions.length}
        isLoading={isLoading}
        searchTerm={searchTerm}
        onSelectExecution={onSelectExecution}
      />
    </div>
  )
}
