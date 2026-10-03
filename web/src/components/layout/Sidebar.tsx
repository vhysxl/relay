import React from 'react'
import {
  LayoutGrid,
  Layers,
  History,
  ChevronsLeft,
  ChevronsRight,
  Database,
  Sparkles,
} from 'lucide-react'
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
  // 3 navigation items without any grouping or branding
  const menuItems = [
    {
      id: 'dashboard' as ViewMode,
      label: 'Dashboard',
      icon: LayoutGrid,
      count: null,
    },
    {
      id: 'workflows' as ViewMode,
      label: 'Workflows',
      icon: Layers,
      count: workflowCount,
    },
    {
      id: 'history' as ViewMode,
      label: 'Execution History',
      icon: History,
      count: executionCount,
    },
  ]

  const renderNavButton = (
    id: ViewMode,
    label: string,
    Icon: React.ComponentType<{ className?: string }>,
    count: number | null
  ) => {
    const isActive =
      currentView === id || (id === 'history' && currentView === 'execution-detail')

    return (
      <button
        key={id}
        onClick={() => onSelectView(id)}
        title={label}
        className={`w-full flex items-center ${
          isCollapsed ? 'justify-center px-2 py-2.5' : 'space-x-3 px-3.5 py-2.5'
        } rounded-xl text-xs transition cursor-pointer select-none ${
          isActive
            ? 'bg-white text-blue-600 font-semibold shadow-xs border border-slate-300/80'
            : 'text-slate-600 hover:text-slate-900 hover:bg-slate-300/40 font-medium'
        }`}
      >
        <Icon
          className={`w-4 h-4 flex-shrink-0 ${
            isActive ? 'text-blue-600 stroke-[2.2]' : 'text-slate-500 stroke-[1.8]'
          }`}
        />
        {!isCollapsed && (
          <>
            <span className="flex-1 text-left truncate">{label}</span>
            {count !== null && (
              <span
                className={`text-[11px] px-2 py-0.5 rounded-full font-semibold transition ${
                  isActive
                    ? 'bg-blue-50 text-blue-600'
                    : 'bg-slate-300/60 text-slate-600'
                }`}
              >
                {count}
              </span>
            )}
          </>
        )}
      </button>
    )
  }

  return (
    <aside
      className={`${
        isCollapsed ? 'w-16' : 'w-56'
      } flex-shrink-0 bg-[#ECEEF2] border-r border-slate-300/80 flex flex-col justify-between select-none z-20 transition-all duration-200`}
    >
      {/* Top Part: Collapse Button + 3 Menu Items (No logo, No grouping header) */}
      <div className="p-3 space-y-3">
        {/* Collapse toggle at top without branding */}
        <div className={`flex items-center ${isCollapsed ? 'justify-center' : 'justify-end'} px-1 pt-1`}>
          <button
            onClick={onToggleCollapse}
            className="p-1.5 rounded-lg text-slate-500 hover:text-slate-800 hover:bg-slate-300/50 transition cursor-pointer"
            title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {isCollapsed ? (
              <ChevronsRight className="w-4 h-4" />
            ) : (
              <ChevronsLeft className="w-4 h-4" />
            )}
          </button>
        </div>

        {/* 3 Sidebar Menu Items */}
        <nav className="space-y-1.5 pt-1">
          {menuItems.map((item) =>
            renderNavButton(item.id, item.label, item.icon, item.count)
          )}
        </nav>
      </div>

      {/* Bottom Part: Dev Mode Controller (No Archive, No Help) */}
      <div className="p-3 border-t border-slate-300/70">
        {!isCollapsed ? (
          <div className="p-2.5 bg-white/90 border border-slate-300/80 rounded-xl space-y-1.5 shadow-2xs">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-semibold text-slate-700 flex items-center gap-1.5">
                {isMockMode ? (
                  <Sparkles className="w-3.5 h-3.5 text-amber-500" />
                ) : (
                  <Database className="w-3.5 h-3.5 text-blue-600" />
                )}
                {isMockMode ? 'Mock Data' : 'Live Engine'}
              </span>
              <span
                className={`w-2 h-2 rounded-full ${
                  isMockMode ? 'bg-amber-500 animate-pulse' : 'bg-emerald-500'
                }`}
              />
            </div>
            <button
              onClick={onToggleMockMode}
              className="w-full py-1 text-[11px] font-medium bg-slate-100 hover:bg-slate-200 border border-slate-200 rounded-lg text-slate-700 transition cursor-pointer shadow-2xs"
            >
              Switch to {isMockMode ? 'Live API' : 'Dummy Data'}
            </button>
          </div>
        ) : (
          <button
            onClick={onToggleMockMode}
            title={isMockMode ? 'Mock Data Active' : 'Live API Active'}
            className="w-full p-2 flex justify-center items-center rounded-lg text-slate-500 hover:bg-slate-300/50 transition cursor-pointer"
          >
            <span
              className={`w-2.5 h-2.5 rounded-full ${
                isMockMode ? 'bg-amber-500 animate-pulse' : 'bg-emerald-500'
              }`}
            />
          </button>
        )}
      </div>
    </aside>
  )
}
