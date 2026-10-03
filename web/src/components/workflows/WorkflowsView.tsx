import React, { useState, useMemo } from 'react'
import { Search, RotateCw, Layers } from 'lucide-react'
import type { Workflow } from '../../types/relay'

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
      {/* Page Header Block matching reference design */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-slate-200/70">
        <div className="space-y-0.5">
          <h1 className="text-xl font-bold tracking-tight text-slate-900">Workflows</h1>
          <p className="text-xs text-slate-500 font-normal">
            List of registered workflows
          </p>
        </div>

        <div className="flex items-center space-x-2">
          <div className="relative">
            <input
              type="text"
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              placeholder="Search workflows..."
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

      {/* Clean Table Card with defined outer border and thin inner borders */}
      <div className="bg-white rounded-lg border-[1.5px] border-slate-300/80 shadow-xs overflow-hidden">
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-slate-100 text-left">
            <thead className="bg-slate-50/75 border-b border-slate-200/80 text-[11px] font-semibold text-slate-500 uppercase tracking-wider">
              <tr>
                <th scope="col" className="px-5 py-3">
                  Name
                </th>
                <th scope="col" className="px-5 py-3">
                  ID
                </th>
                <th scope="col" className="px-5 py-3">
                  Version
                </th>
                <th scope="col" className="px-5 py-3">
                  Steps
                </th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100 bg-white">
              {isLoading && workflows.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-5 py-12 text-center text-slate-400 text-xs">
                    Loading workflows...
                  </td>
                </tr>
              ) : filteredWorkflows.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-5 py-12 text-center text-slate-400 text-xs">
                    {searchTerm
                      ? `No workflows found matching "${searchTerm}".`
                      : 'No registered workflows found.'}
                  </td>
                </tr>
              ) : (
                filteredWorkflows.map((wf) => {
                  const stepCount = wf.steps?.length || 0

                  return (
                    <tr
                      key={wf.id}
                      className="hover:bg-slate-50/70 transition-colors group cursor-pointer"
                    >
                      {/* Name - larger font, hover shows underline */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <div className="flex items-center space-x-3">
                          <div className="w-8 h-8 rounded-md bg-slate-100 flex items-center justify-center text-slate-600 flex-shrink-0 group-hover:bg-slate-200/80 transition-colors">
                            <Layers className="w-4 h-4" />
                          </div>
                          <span className="font-semibold text-sm text-slate-900 group-hover:underline cursor-pointer">
                            {wf.name}
                          </span>
                        </div>
                      </td>

                      {/* ID - larger font */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <span className="font-mono text-sm text-slate-600">
                          {wf.id}
                        </span>
                      </td>

                      {/* Version */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <span className="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-mono font-medium bg-slate-100 text-slate-700 border border-slate-200/80">
                          v{wf.version ?? 1}
                        </span>
                      </td>

                      {/* Steps */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-slate-100 text-slate-800 border border-slate-200/80">
                          {stepCount} {stepCount === 1 ? 'step' : 'steps'}
                        </span>
                      </td>
                    </tr>
                  )
                })
              )}
            </tbody>
          </table>
        </div>

        {/* Footer info */}
        {filteredWorkflows.length > 0 && (
          <div className="px-5 py-2.5 bg-slate-50/40 border-t border-slate-100 flex items-center justify-between text-xs text-slate-500">
            <span>
              Showing <strong>{filteredWorkflows.length}</strong> of{' '}
              <strong>{workflows.length}</strong> workflows
            </span>
          </div>
        )}
      </div>
    </div>
  )
}
