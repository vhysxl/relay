import React from 'react'
import { Search, Bell, Sparkles, User } from 'lucide-react'

interface HeaderProps {
  searchQuery: string
  onSearchChange: (q: string) => void
  isMockMode: boolean
}

export const Header: React.FC<HeaderProps> = ({
  searchQuery,
  onSearchChange,
  isMockMode,
}) => {
  return (
    <header className="h-16 bg-white border-b border-slate-200/80 px-6 flex items-center justify-between z-10 flex-shrink-0">
      {/* Center/Left: Global Search Input matching reference design */}
      <div className="relative w-72 md:w-96">
        <input
          type="text"
          value={searchQuery}
          onChange={(e) => onSearchChange(e.target.value)}
          placeholder="Search queries, personnel, workflows..."
          className="w-full pl-9 pr-4 py-2 bg-slate-100/80 hover:bg-slate-100 focus:bg-white text-xs text-slate-800 placeholder-slate-400 rounded-lg border border-transparent focus:border-slate-300 focus:outline-none focus:ring-2 focus:ring-slate-100 transition shadow-2xs"
        />
        <Search className="w-4 h-4 text-slate-400 absolute left-3 top-2.5" />
      </div>

      {/* Right Side: Notification Bell + User Profile */}
      <div className="flex items-center space-x-4">
        {/* Mock Mode Pill indicator if active */}
        {isMockMode && (
          <span className="hidden sm:inline-flex items-center gap-1.5 px-2.5 py-1 rounded-md text-[11px] font-medium bg-amber-50 text-amber-700 border border-amber-200">
            <Sparkles className="w-3 h-3 text-amber-500" />
            <span>Dev Mock Active</span>
          </span>
        )}

        {/* Notification Bell with Badge */}
        <button
          className="relative p-2 rounded-lg text-slate-400 hover:text-slate-600 hover:bg-slate-100 transition cursor-pointer outline-none focus:outline-none focus:ring-0"
          title="Notifications"
        >
          <Bell className="w-4 h-4" />
          <span className="absolute top-1.5 right-1.5 w-4 h-4 bg-rose-500 text-white text-[9px] font-bold rounded-full flex items-center justify-center border-2 border-white">
            1
          </span>
        </button>

        {/* User Profile Avatar & Info matching reference design */}
        <div className="flex items-center space-x-3 pl-2 border-l border-slate-200/80">
          <div className="w-8 h-8 rounded-full bg-slate-800 text-white flex items-center justify-center font-bold text-xs ring-2 ring-slate-100 flex-shrink-0">
            <User className="w-4 h-4 text-slate-300" />
          </div>
          <div className="hidden md:flex flex-col text-left">
            <span className="text-xs font-semibold text-slate-800 leading-tight">
              Hudson Alvarez
            </span>
            <span className="text-[10px] text-slate-400 font-medium">
              Sr. Executives, HR/Admin
            </span>
          </div>
        </div>
      </div>
    </header>
  )
}
