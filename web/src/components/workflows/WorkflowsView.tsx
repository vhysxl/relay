import React, { useState, useMemo } from 'react'
import { Search, RotateCw, Layers } from 'lucide-react'
import type { Workflow } from '../../types/relay'
import { WorkflowCard } from './WorkflowCard'

interface WorkflowsViewProps {
  workflows: Workflow[]
  isLoading: boolean
  onRefresh: () => void
}

export const WorkflowsView: React.FC<WorkflowsViewProps> = ({
  workflows,
  isLoading,
  onRefresh,
}) => {
  const [searchTerm, setSearchTerm] = useState('')

  const filteredWorkflows = useMemo(() => {
    const q = searchTerm.trim().toLowerCase()
    if (!q) return workflows
    return workflows.filter(
      (w) =>
        (w.name && w.name.toLowerCase().includes(q)) ||
        (w.id && w.id.toLowerCase().includes(q))
    )
  }, [workflows, searchTerm])

  return (
    <div className="space-y-4">
      {/* Header Block with Search & Refresh */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-slate-200/70">
        <div className="space-y-0.5">
          <div className="flex items-center space-x-2.5">
            <h1 className="text-xl font-bold tracking-tight text-slate-900">Workflows</h1>
          </div>
          <p className="text-xs text-slate-500 font-normal">
            Registered workflow definitions and DAG step sequences.
          </p>
        </div>

        <div className="flex items-center space-x-2">
          <div className="relative">
            <input
              type="text"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder="Search registered workflows..."
              className="pl-8 pr-3 py-1.5 text-xs bg-white border border-slate-200 rounded-lg focus:outline-none focus:ring-1 focus:ring-slate-400 focus:border-slate-400 w-56 text-slate-700 placeholder-slate-400 shadow-xs"
            />
            <Search className="w-3.5 h-3.5 text-slate-400 absolute left-2.5 top-2.5" />
          </div>

          <button
            onClick={onRefresh}
            disabled={isLoading}
            className="px-3 py-1.5 bg-white border border-slate-200 hover:bg-slate-50 text-slate-700 text-xs font-semibold rounded-lg shadow-xs flex items-center space-x-1.5 transition disabled:opacity-50 cursor-pointer outline-none focus:outline-none focus:ring-0"
          >
            <RotateCw className={`w-3.5 h-3.5 text-slate-500 ${isLoading ? 'animate-spin' : ''}`} />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Dynamic Workflows List */}
      <div className="space-y-3.5">
        {isLoading && workflows.length === 0 ? (
          <div className="text-center py-8 text-slate-400 text-xs">
            Loading registered workflows from Relay engine...
          </div>
        ) : filteredWorkflows.length === 0 ? (
          <div className="bg-white rounded-xl border border-dashed border-slate-300 p-8 text-center space-y-2">
            <div className="w-10 h-10 rounded-full bg-slate-100 text-slate-400 mx-auto flex items-center justify-center">
              <Layers className="w-5 h-5 text-slate-400" />
            </div>
            <div className="space-y-0.5">
              <h3 className="text-sm font-semibold text-slate-900">
                {searchTerm ? 'No Matching Workflows' : 'No Workflows Registered'}
              </h3>
              <p className="text-[11px] text-slate-500 max-w-sm mx-auto">
                {searchTerm
                  ? `No workflow definition matches "${searchTerm}".`
                  : 'No registered workflow definitions found. Register workflows using the Relay REST API (POST /workflows).'}
              </p>
            </div>
          </div>
        ) : (
          filteredWorkflows.map((wf) => <WorkflowCard key={wf.id} workflow={wf} />)
        )}
      </div>
    </div>
  )
}
