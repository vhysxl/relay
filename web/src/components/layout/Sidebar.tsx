import React from 'react'
import { Layers, History, PanelLeftClose, PanelLeft, Database, Sparkles } from 'lucide-react'
import type { ViewMode } from '../../types/relay'

interface SidebarProps {
  currentView: ViewMode
  onSelectView: (view: ViewMode) => void
  workflowCount: number
  executionCount: number
  isCollapsed: boolean
  onToggleCollapse: () => void
  isMockMode: boolean
  onToggleMockMode: () => void
}

export const Sidebar: React.FC<SidebarProps> = ({
  currentView,
  onSelectView,
  workflowCount,
  executionCount,
  isCollapsed,
  onToggleCollapse,
  isMockMode,
  onToggleMockMode,
}) => {
  const isWorkflowsActive = currentView === 'workflows'
  const isHistoryActive = currentView === 'history' || currentView === 'execution-detail'

  return (
    <aside
      className={`${
        isCollapsed ? 'w-16' : 'w-64'
      } flex-shrink-0 bg-white border-r border-slate-200/90 flex flex-col justify-between select-none z-20 transition-all duration-200`}
    >
      <div className="p-4 space-y-6">
        {/* App Brand Header */}
        <div className="flex items-center justify-between px-1 pt-1">
          {!isCollapsed && (
            <div className="flex items-center space-x-2.5">
              <span className="font-bold text-lg text-slate-900 tracking-tight">Relay</span>
            </div>
          )}
          <button
            onClick={onToggleCollapse}
            className="p-1 rounded-md text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition mx-auto cursor-pointer"
            title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {isCollapsed ? <PanelLeft className="w-4 h-4" /> : <PanelLeftClose className="w-4 h-4" />}
          </button>
        </div>

        {/* Navigation */}
        <nav className="space-y-1.5">
          <button
            onClick={() => onSelectView('workflows')}
            className={`w-full flex items-center ${
              isCollapsed ? 'justify-center px-2' : 'space-x-3 px-3'
            } py-2 text-sm font-medium rounded-xl transition cursor-pointer ${
              isWorkflowsActive
                ? 'text-slate-900 bg-slate-100/90 shadow-xs'
                : 'text-slate-600 hover:text-slate-900 hover:bg-slate-50'
            }`}
            title="Workflows"
          >
            <Layers
              className={`w-4 h-4 flex-shrink-0 ${
                isWorkflowsActive ? 'text-emerald-600' : 'text-slate-400'
              }`}
            />
            {!isCollapsed && (
              <>
                <span className={`flex-1 text-left ${isWorkflowsActive ? 'font-semibold' : 'font-medium'}`}>
                  Workflows
                </span>
                <span className="bg-slate-100 text-slate-700 text-xs px-2 py-0.5 rounded-full font-semibold">
                  {workflowCount}
                </span>
              </>
            )}
          </button>

          <button
            onClick={() => onSelectView('history')}
            className={`w-full flex items-center ${
              isCollapsed ? 'justify-center px-2' : 'space-x-3 px-3'
            } py-2 text-sm font-medium rounded-xl transition cursor-pointer ${
              isHistoryActive
                ? 'text-slate-900 bg-slate-100/90 shadow-xs'
                : 'text-slate-600 hover:text-slate-900 hover:bg-slate-50'
            }`}
            title="Execution History"
          >
            <History
              className={`w-4 h-4 flex-shrink-0 ${
                isHistoryActive ? 'text-emerald-600' : 'text-slate-400'
              }`}
            />
            {!isCollapsed && (
              <>
                <span className={`flex-1 text-left ${isHistoryActive ? 'font-semibold' : 'font-medium'}`}>
                  Execution History
                </span>
                <span className="bg-slate-100 text-slate-600 text-xs px-2 py-0.5 rounded-full font-medium">
                  {executionCount}
                </span>
              </>
            )}
          </button>
        </nav>
      </div>

      {/* Dev Mode: Mock Data Switcher at bottom */}
      <div className="p-3 border-t border-slate-100">
        {!isCollapsed ? (
          <div className="p-3 bg-slate-50 border border-slate-200/80 rounded-xl space-y-2">
            <div className="flex items-center justify-between">
              <span className="text-xs font-semibold text-slate-800 flex items-center gap-1.5">
                {isMockMode ? (
                  <Sparkles className="w-3.5 h-3.5 text-amber-500" />
                ) : (
                  <Database className="w-3.5 h-3.5 text-emerald-600" />
                )}
                {isMockMode ? 'Mock Data Active' : 'Live API Active'}
              </span>
              <span
                className={`w-2 h-2 rounded-full ${
                  isMockMode ? 'bg-amber-500 animate-pulse' : 'bg-emerald-500'
                }`}
              />
            </div>
            <p className="text-[11px] text-slate-500 leading-tight">
              {isMockMode
                ? 'Dev preview with mock DAG workflows & executions.'
                : 'Querying backend REST API directly.'}
            </p>
            <button
              onClick={onToggleMockMode}
              className="w-full py-1 text-xs font-semibold bg-white hover:bg-slate-100 border border-slate-200 rounded-lg text-slate-700 transition cursor-pointer shadow-xs"
            >
              Switch to {isMockMode ? 'Live API' : 'Dummy Data'}
            </button>
          </div>
        ) : (
          <button
            onClick={onToggleMockMode}
            title={isMockMode ? 'Mock Data Active (Click to switch)' : 'Live API Active (Click to switch)'}
            className="w-full p-2 flex justify-center items-center rounded-lg text-slate-500 hover:bg-slate-100 transition cursor-pointer"
          >
            <span
              className={`w-3 h-3 rounded-full ${
                isMockMode ? 'bg-amber-500 animate-pulse' : 'bg-emerald-500'
              }`}
            />
          </button>
        )}
      </div>
    </aside>
  )
}
