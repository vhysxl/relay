import React from 'react'
import {
  LayoutGrid,
  Layers,
  History,
  Settings,
  Archive,
  HelpCircle,
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
  // Navigation items grouped by category as seen in reference design
  const generalItems = [
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
  ]

  const managementItems = [
    {
      id: 'history' as ViewMode,
      label: 'Execution History',
      icon: History,
      count: executionCount,
    },
  ]

  const settingsItems = [
    {
      id: 'settings' as ViewMode,
      label: 'Settings',
      icon: Settings,
      count: null,
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
            ? 'bg-white text-blue-600 font-semibold shadow-xs border border-slate-200/90'
            : 'text-slate-600 hover:text-slate-900 hover:bg-slate-100/70 font-medium'
        }`}
      >
        <Icon
          className={`w-4 h-4 flex-shrink-0 ${
            isActive ? 'text-blue-600 stroke-[2.2]' : 'text-slate-400 stroke-[1.8]'
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
                    : 'bg-slate-100 text-slate-500'
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
        isCollapsed ? 'w-16' : 'w-60'
      } flex-shrink-0 bg-[#F9FAFB] border-r border-slate-200/80 flex flex-col justify-between select-none z-20 transition-all duration-200`}
    >
      {/* Top Part: Brand + Navigation Sections */}
      <div className="p-3.5 space-y-4">
        {/* Brand Header (Custom Relay Identity, Excluding 'Your Company') */}
        <div className="flex items-center justify-between px-1.5 py-1">
          {!isCollapsed && (
            <div className="flex items-center space-x-2.5">
              <div className="w-7 h-7 rounded-lg bg-blue-600 flex items-center justify-center shadow-xs flex-shrink-0">
                <svg
                  className="w-4 h-4 text-white"
                  viewBox="0 0 24 24"
                  fill="none"
                  stroke="currentColor"
                  strokeWidth="2.5"
                  strokeLinecap="round"
                  strokeLinejoin="round"
                >
                  <path d="M13 2L3 14H12L11 22L21 10H12L13 2Z" fill="currentColor" fillOpacity="0.2" />
                </svg>
              </div>
              <div className="flex flex-col">
                <span className="font-bold text-[15px] text-slate-900 tracking-tight leading-none">
                  Relay
                </span>
                <span className="text-[10px] text-slate-400 font-medium tracking-normal mt-0.5">
                  Task Orchestrator
                </span>
              </div>
            </div>
          )}

          {/* Minimal Collapse Button */}
          <button
            onClick={onToggleCollapse}
            className="p-1 rounded-md text-slate-400 hover:text-slate-600 hover:bg-slate-200/60 transition cursor-pointer mx-auto"
            title={isCollapsed ? 'Expand sidebar' : 'Collapse sidebar'}
          >
            {isCollapsed ? (
              <ChevronsRight className="w-4 h-4" />
            ) : (
              <ChevronsLeft className="w-4 h-4" />
            )}
          </button>
        </div>

        {/* Navigation Sections */}
        <div className="space-y-4 pt-1">
          {/* GENERAL Category */}
          <div className="space-y-1">
            {!isCollapsed && (
              <div className="px-3 text-[10px] font-bold uppercase tracking-wider text-slate-400">
                General
              </div>
            )}
            <div className="space-y-0.5">
              {generalItems.map((item) =>
                renderNavButton(item.id, item.label, item.icon, item.count)
              )}
            </div>
          </div>

          {/* MANAGEMENT Category */}
          <div className="space-y-1">
            {!isCollapsed && (
              <div className="px-3 text-[10px] font-bold uppercase tracking-wider text-slate-400">
                Management
              </div>
            )}
            <div className="space-y-0.5">
              {managementItems.map((item) =>
                renderNavButton(item.id, item.label, item.icon, item.count)
              )}
            </div>
          </div>

          {/* SETTINGS Category */}
          <div className="space-y-1">
            {!isCollapsed && (
              <div className="px-3 text-[10px] font-bold uppercase tracking-wider text-slate-400">
                Settings
              </div>
            )}
            <div className="space-y-0.5">
              {settingsItems.map((item) =>
                renderNavButton(item.id, item.label, item.icon, item.count)
              )}
            </div>
          </div>
        </div>
      </div>

      {/* Bottom Part: Archive, Help & Dev Mode Controller */}
      <div className="p-3 border-t border-slate-200/70 space-y-2">
        {/* Footer items matching reference design */}
        <div className="space-y-0.5">
          <button
            onClick={() => onSelectView('history')}
            className={`w-full flex items-center ${
              isCollapsed ? 'justify-center px-2 py-2' : 'space-x-3 px-3 py-2'
            } rounded-xl text-xs text-slate-500 hover:text-slate-800 hover:bg-slate-200/50 transition cursor-pointer`}
            title="Archive"
          >
            <Archive className="w-4 h-4 text-slate-400 flex-shrink-0" />
            {!isCollapsed && <span className="font-medium">Archive</span>}
          </button>

          <a
            href="https://github.com/vhysxl/relay#readme"
            target="_blank"
            rel="noreferrer"
            className={`w-full flex items-center ${
              isCollapsed ? 'justify-center px-2 py-2' : 'space-x-3 px-3 py-2'
            } rounded-xl text-xs text-slate-500 hover:text-slate-800 hover:bg-slate-200/50 transition cursor-pointer`}
            title="Help & Docs"
          >
            <HelpCircle className="w-4 h-4 text-slate-400 flex-shrink-0" />
            {!isCollapsed && <span className="font-medium">Help & Docs</span>}
          </a>
        </div>

        {/* Dev Mode Data Switcher Pill */}
        {!isCollapsed ? (
          <div className="p-2.5 bg-white border border-slate-200/90 rounded-xl space-y-1.5 shadow-2xs">
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
              className="w-full py-1 text-[11px] font-medium bg-slate-50 hover:bg-slate-100 border border-slate-200 rounded-lg text-slate-700 transition cursor-pointer shadow-2xs"
            >
              Switch to {isMockMode ? 'Live API' : 'Dummy Data'}
            </button>
          </div>
        ) : (
          <button
            onClick={onToggleMockMode}
            title={isMockMode ? 'Mock Data Active (Click to switch)' : 'Live API Active (Click to switch)'}
            className="w-full p-2 flex justify-center items-center rounded-lg text-slate-500 hover:bg-slate-200/50 transition cursor-pointer"
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
