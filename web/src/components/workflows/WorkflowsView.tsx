import React, { useState, useMemo } from 'react'
import { Search, RotateCw } from 'lucide-react'
import type { Workflow } from '../../types/relay'

interface WorkflowsViewProps {
  workflows: Workflow[]
  isLoading: boolean
  onRefresh: () => void
  onSelectWorkflow: (id: string) => void
}

export const WorkflowsView: React.FC<WorkflowsViewProps> = ({
  workflows,
  isLoading,
  onRefresh,
  onSelectWorkflow,
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
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-neutral-800">
        <div className="space-y-0.5">
          <h1 className="text-xl font-bold tracking-tight text-neutral-100">Workflows</h1>
          <p className="text-xs text-neutral-400 font-normal">
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

      {/* Clean Table Card with defined outer border and thin inner borders */}
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden">
        <div className="overflow-x-auto">
          <table className="min-w-full divide-y divide-neutral-800/80 text-left">
            <thead className="bg-[#181818] border-b border-neutral-800 text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
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
            <tbody className="divide-y divide-neutral-800/80 bg-[#141414]">
              {isLoading && workflows.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-5 py-12 text-center text-neutral-500 text-xs">
                    Loading workflows...
                  </td>
                </tr>
              ) : filteredWorkflows.length === 0 ? (
                <tr>
                  <td colSpan={4} className="px-5 py-12 text-center text-neutral-500 text-xs">
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
                      onClick={() => onSelectWorkflow(wf.id)}
                      className="hover:bg-neutral-800/40 transition-colors group cursor-pointer"
                    >
                      {/* Name - hover shows underline */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <span className="font-medium text-sm text-neutral-200 group-hover:underline cursor-pointer">
                          {wf.name}
                        </span>
                      </td>

                      {/* ID */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <span className="font-mono text-sm text-neutral-400">
                          {wf.id}
                        </span>
                      </td>

                      {/* Version */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <span className="inline-flex items-center px-2 py-0.5 rounded-md text-xs font-mono font-medium bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                          v{wf.version ?? 1}
                        </span>
                      </td>

                      {/* Steps */}
                      <td className="px-5 py-3 whitespace-nowrap">
                        <span className="inline-flex items-center px-2.5 py-0.5 rounded-md text-xs font-medium bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
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
          <div className="px-5 py-2.5 bg-[#181818] border-t border-neutral-800 flex items-center justify-between text-xs text-neutral-400">
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
