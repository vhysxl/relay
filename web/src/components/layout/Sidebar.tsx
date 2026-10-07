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
  // 3 navigation items without grouping or branding
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
      currentView === id ||
      (id === 'history' && currentView === 'execution-detail') ||
      (id === 'workflows' && currentView === 'workflow-detail')

    return (
      <button
        key={id}
        onClick={() => onSelectView(id)}
        title={label}
        className={`w-full flex items-center ${
          isCollapsed ? 'justify-center px-2 py-2.5' : 'space-x-3 px-3.5 py-2.5'
        } rounded-lg text-xs transition-all duration-150 cursor-pointer select-none outline-none focus:outline-none focus:ring-0 ${
          isActive
            ? 'bg-neutral-800 text-neutral-100 font-semibold shadow-xs border border-neutral-700'
            : 'text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800/40 font-medium border border-transparent'
        }`}
      >
        <Icon
          className={`w-4 h-4 flex-shrink-0 transition-colors ${
            isActive ? 'text-neutral-100 stroke-[2.2]' : 'text-neutral-500 stroke-[1.8]'
          }`}
        />
        {!isCollapsed && (
          <>
            <span className="flex-1 text-left truncate">{label}</span>
            {count !== null && (
              <span
                className={`text-[11px] px-2 py-0.5 rounded-md font-semibold transition-colors ${
                  isActive
                    ? 'bg-neutral-700 text-neutral-100'
                    : 'bg-neutral-800/90 text-neutral-400'
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
      } flex-shrink-0 bg-[#0e0e0e] border-r border-neutral-800 flex flex-col justify-between select-none z-20 transition-all duration-200`}
    >
      {/* Top Part: Collapse Button + 3 Menu Items (No logo, No grouping header) */}
      <div className="p-3 space-y-3">
        {/* Collapse toggle at top without branding */}
        <div className={`flex items-center ${isCollapsed ? 'justify-center' : 'justify-end'} px-1 pt-1`}>
          <button
            onClick={onToggleCollapse}
            className="p-1.5 rounded-md text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800/60 transition cursor-pointer outline-none focus:outline-none focus:ring-0"
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

      {/* Bottom Part: Dev Mode Controller with subtle styling */}
      <div className="p-3 border-t border-neutral-800">
        {!isCollapsed ? (
          <div className="p-2.5 bg-[#141414] border border-neutral-800 rounded-lg space-y-1.5 shadow-2xs">
            <div className="flex items-center justify-between">
              <span className="text-[11px] font-semibold text-neutral-300 flex items-center gap-1.5">
                {isMockMode ? (
                  <Sparkles className="w-3.5 h-3.5 text-amber-500" />
                ) : (
                  <Database className="w-3.5 h-3.5 text-neutral-300" />
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
              className="w-full py-1 text-[11px] font-medium bg-neutral-800 hover:bg-neutral-700/80 border border-neutral-700/80 rounded-lg text-neutral-200 transition cursor-pointer shadow-2xs outline-none focus:outline-none focus:ring-0"
            >
              Switch to {isMockMode ? 'Live API' : 'Dummy Data'}
            </button>
          </div>
        ) : (
          <button
            onClick={onToggleMockMode}
            title={isMockMode ? 'Mock Data Active (Click to switch)' : 'Live API Active (Click to switch)'}
            className="w-full p-2 flex justify-center items-center rounded-lg text-neutral-400 hover:bg-neutral-800/60 transition cursor-pointer outline-none focus:outline-none focus:ring-0"
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
