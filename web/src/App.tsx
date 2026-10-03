import React, { useState, useEffect, useCallback } from 'react'
import type { Workflow, Execution, ViewMode } from './types/relay'
import { Sidebar } from './components/layout/Sidebar'
import { Header } from './components/layout/Header'
import { DashboardView } from './components/dashboard/DashboardView'
import { WorkflowsView } from './components/workflows/WorkflowsView'
import { HistoryView } from './components/history/HistoryView'
import { ExecutionDetailView } from './components/execution-detail/ExecutionDetailView'
import { SettingsView } from './components/settings/SettingsView'
import { mockWorkflows, mockExecutions } from './data/mockData'

export const App: React.FC = () => {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [executions, setExecutions] = useState<Execution[]>([])
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [currentView, setCurrentView] = useState<ViewMode>('dashboard')
  const [selectedExecutionId, setSelectedExecutionId] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(false)
  const [globalSearch, setGlobalSearch] = useState<string>('')

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
      }, 100)
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

      if (hasLiveWorkflows || hasLiveExecutions) {
        setExecutions(Array.isArray(execData) ? execData : [])
        setWorkflows(Array.isArray(wfData) ? wfData : [])
      } else {
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
    } else if (hash === 'workflows') {
      setSelectedExecutionId(null)
      setCurrentView('workflows')
    } else if (hash === 'settings') {
      setSelectedExecutionId(null)
      setCurrentView('settings')
    } else {
      setSelectedExecutionId(null)
      setCurrentView('dashboard')
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
    } else if (view === 'workflows') {
      window.location.hash = 'workflows'
    } else if (view === 'settings') {
      window.location.hash = 'settings'
    } else {
      window.location.hash = 'dashboard'
    }
  }

  return (
    <div className="text-slate-800 bg-[#F4F6F9] flex h-screen overflow-hidden antialiased">
      {/* Refactored Clean Left Sidebar */}
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

      {/* Main Content Workspace with Top Header */}
      <div className="flex-1 flex flex-col h-screen overflow-hidden bg-[#F4F6F9]">
        {/* Top Header Bar */}
        <Header
          searchQuery={globalSearch}
          onSearchChange={setGlobalSearch}
          isMockMode={isMockMode}
        />

        {/* Workspace Body */}
        <main className="flex-1 overflow-y-auto px-8 py-6 space-y-5">
          {currentView === 'dashboard' && (
            <DashboardView
              workflows={workflows}
              executions={executions}
              onNavigate={navigateToView}
            />
          )}

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

          {currentView === 'settings' && (
            <SettingsView
              isMockMode={isMockMode}
              onToggleMockMode={handleToggleMockMode}
            />
          )}
        </main>
      </div>
    </div>
  )
}

export default App
