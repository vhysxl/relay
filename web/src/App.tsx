import React, { useState, useEffect, useCallback } from 'react'
import type { Workflow, Execution, ViewMode } from './types/relay'
import { Sidebar } from './components/layout/Sidebar'
import { WorkflowsView } from './components/workflows/WorkflowsView'
import { HistoryView } from './components/history/HistoryView'
import { ExecutionDetailView } from './components/execution-detail/ExecutionDetailView'

export const App: React.FC = () => {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [executions, setExecutions] = useState<Execution[]>([])
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [currentView, setCurrentView] = useState<ViewMode>('workflows')
  const [selectedExecutionId, setSelectedExecutionId] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(false)

  // Fetch workflows and executions
  const loadData = useCallback(async () => {
    setIsLoading(true)
    try {
      const [execRes, wfRes] = await Promise.all([
        fetch('/executions'),
        fetch('/workflows'),
      ])

      const execData = execRes.ok ? await execRes.json() : []
      const wfData = wfRes.ok ? await wfRes.json() : []

      setExecutions(Array.isArray(execData) ? execData : [])
      setWorkflows(Array.isArray(wfData) ? wfData : [])
    } catch (err) {
      console.error('Failed to load Relay data:', err)
    } finally {
      setIsLoading(false)
    }
  }, [])

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
      />

      {/* Main Content Workspace */}
      <div className="flex-1 flex flex-col h-screen overflow-hidden bg-slate-50/70">
        <main className="flex-1 overflow-y-auto px-8 py-6 space-y-5">
          {currentView === 'workflows' && (
            <WorkflowsView
              workflows={workflows}
              isLoading={isLoading}
              onRefresh={loadData}
            />
          )}

          {currentView === 'history' && (
            <HistoryView
              executions={executions}
              isLoading={isLoading}
              onRefresh={loadData}
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
