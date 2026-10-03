import React, { useState, useEffect, useCallback } from 'react'
import type { Workflow, Execution, ViewMode } from './types/relay'
import { Sidebar } from './components/layout/Sidebar'
import { WorkflowsView } from './components/workflows/WorkflowsView'
import { HistoryView } from './components/history/HistoryView'
import { ExecutionDetailView } from './components/execution-detail/ExecutionDetailView'
import { mockWorkflows, mockExecutions } from './data/mockData'
import { Sparkles } from 'lucide-react'

export const App: React.FC = () => {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [executions, setExecutions] = useState<Execution[]>([])
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [currentView, setCurrentView] = useState<ViewMode>('workflows')
  const [selectedExecutionId, setSelectedExecutionId] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(false)

  // Default to mock data mode during dev so UI is always previewable
  const [isMockMode, setIsMockMode] = useState<boolean>(() => {
    const saved = localStorage.getItem('relay_mock_mode')
    return saved !== null ? saved === 'true' : true
  })

  // Fetch workflows and executions or load mock data
  const loadData = useCallback(async (forceMock = isMockMode) => {
    setIsLoading(true)

    if (forceMock) {
      setTimeout(() => {
        setWorkflows(mockWorkflows)
        setExecutions(mockExecutions)
        setIsLoading(false)
      }, 150)
      return
    }

    try {
      const [execRes, wfRes] = await Promise.all([
        fetch('/executions').catch(() => null),
        fetch('/workflows').catch(() => null),
      ])

      const execData = execRes && execRes.ok ? await execRes.json().catch(() => []) : []
      const wfData = wfRes && wfRes.ok ? await wfRes.json().catch(() => []) : []

      const hasLiveWorkflows = Array.isArray(wfData) && wfData.length > 0
      const hasLiveExecutions = Array.isArray(execData) && execData.length > 0

      // If backend returns data, use it; otherwise fallback to mock data
      if (hasLiveWorkflows || hasLiveExecutions) {
        setExecutions(Array.isArray(execData) ? execData : [])
        setWorkflows(Array.isArray(wfData) ? wfData : [])
      } else {
        // Fallback to mock data if DB is empty / offline
        setWorkflows(mockWorkflows)
        setExecutions(mockExecutions)
        setIsMockMode(true)
        localStorage.setItem('relay_mock_mode', 'true')
      }
    } catch (err) {
      console.warn('Backend unavailable, falling back to mock data:', err)
      setWorkflows(mockWorkflows)
      setExecutions(mockExecutions)
      setIsMockMode(true)
      localStorage.setItem('relay_mock_mode', 'true')
    } finally {
      setIsLoading(false)
    }
  }, [isMockMode])

  const handleToggleMockMode = () => {
    const nextMode = !isMockMode
    setIsMockMode(nextMode)
    localStorage.setItem('relay_mock_mode', String(nextMode))
    loadData(nextMode)
  }

  // Handle URL hash changes for deep linking & back button support
  const handleHashChange = useCallback(() => {
    const hash = window.location.hash.replace(/^#\/?/, '')
    if (hash.startsWith('execution/')) {
      const id = hash.replace('execution/', '')
      setSelectedExecutionId(id)
      setCurrentView('execution-detail')
    } else if (hash === 'history' || window.location.pathname.includes('/history')) {
      setSelectedExecutionId(null)
      setCurrentView('history')
    } else {
      setSelectedExecutionId(null)
      setCurrentView('workflows')
    }
  }, [])

  useEffect(() => {
    handleHashChange()
    window.addEventListener('hashchange', handleHashChange)
    loadData()

    return () => {
      window.removeEventListener('hashchange', handleHashChange)
    }
  }, [handleHashChange, loadData])

  const navigateToView = (view: ViewMode, executionId?: string) => {
    if (view === 'execution-detail' && executionId) {
      window.location.hash = `execution/${executionId}`
    } else if (view === 'history') {
      window.location.hash = 'history'
    } else {
      window.location.hash = 'workflows'
    }
  }

  return (
    <div className="text-slate-800 bg-[#f8fafc] flex h-screen overflow-hidden antialiased">
      {/* Left Sidebar */}
      <Sidebar
        currentView={currentView}
        onSelectView={(v) => navigateToView(v)}
        workflowCount={workflows.length}
        executionCount={executions.length}
        isCollapsed={isSidebarCollapsed}
        onToggleCollapse={() => setIsSidebarCollapsed(!isSidebarCollapsed)}
        isMockMode={isMockMode}
        onToggleMockMode={handleToggleMockMode}
      />

      {/* Main Content Workspace */}
      <div className="flex-1 flex flex-col h-screen overflow-hidden bg-slate-50/70">
        {/* Top Info Banner when in Mock Data Mode */}
        {isMockMode && (
          <div className="bg-amber-50/90 border-b border-amber-200/80 px-6 py-1.5 flex items-center justify-between text-xs text-amber-800 select-none">
            <div className="flex items-center space-x-2">
              <Sparkles className="w-3.5 h-3.5 text-amber-600 flex-shrink-0" />
              <span>
                <strong>Frontend Dev Mode Active:</strong> Using idempotent dummy workflows & executions. Database connection is optional.
              </span>
            </div>
            <button
              onClick={handleToggleMockMode}
              className="text-[11px] underline font-medium hover:text-amber-900 cursor-pointer"
            >
              Try Live Server API
            </button>
          </div>
        )}

        <main className="flex-1 overflow-y-auto px-8 py-6 space-y-5">
          {currentView === 'workflows' && (
            <WorkflowsView
              workflows={workflows}
              isLoading={isLoading}
              onRefresh={() => loadData()}
            />
          )}

          {currentView === 'history' && (
            <HistoryView
              executions={executions}
              isLoading={isLoading}
              onRefresh={() => loadData()}
              onSelectExecution={(id) => navigateToView('execution-detail', id)}
            />
          )}

          {currentView === 'execution-detail' && selectedExecutionId && (
            <ExecutionDetailView
              executionId={selectedExecutionId}
              onBack={() => navigateToView('history')}
            />
          )}
        </main>
      </div>
    </div>
  )
}

export default App
