import React from 'react'
import { Database, Cpu, Sparkles } from 'lucide-react'

interface SettingsViewProps {
  isMockMode: boolean
  onToggleMockMode: () => void
}

export const SettingsView: React.FC<SettingsViewProps> = ({
  isMockMode,
  onToggleMockMode,
}) => {
  return (
    <div className="space-y-6 max-w-4xl">
      {/* Header */}
      <div className="pb-1 border-b border-slate-200/70">
        <h1 className="text-2xl font-bold tracking-tight text-slate-900">
          Relay Engine Settings
        </h1>
        <p className="text-xs text-slate-500 font-normal mt-0.5">
          Configuration, broker connections, and developer environment modes.
        </p>
      </div>

      {/* Cards List */}
      <div className="space-y-4">
        {/* Card 1: Developer Environment Mode */}
        <div className="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-xs space-y-4">
          <div className="flex items-start justify-between">
            <div className="flex items-center space-x-3">
              <div className="w-9 h-9 rounded-xl bg-amber-50 text-amber-600 flex items-center justify-center flex-shrink-0">
                <Sparkles className="w-5 h-5" />
              </div>
              <div>
                <h3 className="text-sm font-bold text-slate-900">
                  Developer Mock Data Mode
                </h3>
                <p className="text-xs text-slate-500">
                  Enable offline DAG workflows and executions with rich dummy data.
                </p>
              </div>
            </div>

            <button
              onClick={onToggleMockMode}
              className={`px-4 py-1.5 text-xs font-semibold rounded-xl transition cursor-pointer ${
                isMockMode
                  ? 'bg-amber-500 hover:bg-amber-600 text-white shadow-xs'
                  : 'bg-slate-100 hover:bg-slate-200 text-slate-700'
              }`}
            >
              {isMockMode ? 'Mock Mode ON' : 'Mock Mode OFF'}
            </button>
          </div>
        </div>

        {/* Card 2: Database Connection */}
        <div className="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-xs space-y-4">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-blue-50 text-blue-600 flex items-center justify-center flex-shrink-0">
              <Database className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-900">MySQL InnoDB Engine</h3>
              <p className="text-xs text-slate-500">
                Primary persistence with atomic distributed row locking (SKIP LOCKED).
              </p>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-3 gap-3 text-xs pt-1">
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-200/60 space-y-1">
              <span className="text-slate-400 font-medium">Default Port</span>
              <p className="font-mono font-semibold text-slate-800">3306 (TCP)</p>
            </div>
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-200/60 space-y-1">
              <span className="text-slate-400 font-medium">Max Open Conns</span>
              <p className="font-mono font-semibold text-slate-800">25 connections</p>
            </div>
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-200/60 space-y-1">
              <span className="text-slate-400 font-medium">Max Idle Conns</span>
              <p className="font-mono font-semibold text-slate-800">10 connections</p>
            </div>
          </div>
        </div>

        {/* Card 3: Worker Pool Engine */}
        <div className="bg-white rounded-2xl border border-slate-200/80 p-5 shadow-xs space-y-4">
          <div className="flex items-center space-x-3">
            <div className="w-9 h-9 rounded-xl bg-indigo-50 text-indigo-600 flex items-center justify-center flex-shrink-0">
              <Cpu className="w-5 h-5" />
            </div>
            <div>
              <h3 className="text-sm font-bold text-slate-900">Worker Pool Runner</h3>
              <p className="text-xs text-slate-500">
                Concurrent goroutine pool claiming steps with exponential backoff.
              </p>
            </div>
          </div>

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 text-xs pt-1">
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-200/60 space-y-1">
              <span className="text-slate-400 font-medium">Worker Count</span>
              <p className="font-mono font-semibold text-slate-800">3 concurrent workers</p>
            </div>
            <div className="p-3 bg-slate-50 rounded-xl border border-slate-200/60 space-y-1">
              <span className="text-slate-400 font-medium">Recovery Mode</span>
              <p className="font-mono font-semibold text-slate-800">Automatic panic catch</p>
            </div>
          </div>
        </div>
      </div>
    </div>
  )
}
