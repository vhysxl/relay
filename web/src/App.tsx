import React, { useState, useEffect, useCallback } from 'react'
import { Cpu, HardDrive } from 'lucide-react'
import type { Workflow, Execution, ViewMode, SystemMetrics } from './types/relay'
import { Sidebar } from './components/layout/Sidebar'
import { DashboardView } from './components/dashboard/DashboardView'
import { WorkflowsView } from './components/workflows/WorkflowsView'
import { HistoryView } from './components/history/HistoryView'
import { ExecutionDetailView } from './components/execution-detail/ExecutionDetailView'
import { WorkflowDetailView } from './components/workflows/WorkflowDetailView'
import { mockWorkflows, mockExecutions, mockSystemMetrics } from './data/mockData'

export const App: React.FC = () => {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [executions, setExecutions] = useState<Execution[]>([])
  const [systemMetrics, setSystemMetrics] = useState<SystemMetrics>(mockSystemMetrics)
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [currentView, setCurrentView] = useState<ViewMode>('dashboard')
  const [selectedExecutionId, setSelectedExecutionId] = useState<string | null>(null)
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(false)

  // Default to mock data mode during dev so UI is always previewable
  const [isMockMode, setIsMockMode] = useState<boolean>(() => {
    const saved = localStorage.getItem('relay_mock_mode')
    return saved !== null ? saved === 'true' : true
  })

  // Fetch real-time system metrics (CPU, RAM, Goroutines)
  const fetchSystemMetrics = useCallback(async () => {
    try {
      const res = await fetch('/system/metrics').catch(() => null)
      if (res && res.ok) {
        const data: SystemMetrics = await res.json().catch(() => null)
        if (data) {
          setSystemMetrics(data)
          return
        }
      }
      setSystemMetrics(mockSystemMetrics)
    } catch {
      setSystemMetrics(mockSystemMetrics)
    }
  }, [])

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
      setSelectedWorkflowId(null)
      setCurrentView('execution-detail')
    } else if (hash.startsWith('workflows/')) {
      const id = hash.replace('workflows/', '')
      setSelectedWorkflowId(id)
      setSelectedExecutionId(null)
      setCurrentView('workflow-detail')
    } else if (hash.startsWith('workflow/')) {
      const id = hash.replace('workflow/', '')
      setSelectedWorkflowId(id)
      setSelectedExecutionId(null)
      setCurrentView('workflow-detail')
    } else if (hash === 'history' || window.location.pathname.includes('/history')) {
      setSelectedExecutionId(null)
      setSelectedWorkflowId(null)
      setCurrentView('history')
    } else if (hash === 'workflows') {
      setSelectedExecutionId(null)
      setSelectedWorkflowId(null)
      setCurrentView('workflows')
    } else {
      setSelectedExecutionId(null)
      setSelectedWorkflowId(null)
      setCurrentView('dashboard')
    }
  }, [])

  useEffect(() => {
    handleHashChange()
    window.addEventListener('hashchange', handleHashChange)
    loadData()
    fetchSystemMetrics()

    const metricsTimer = setInterval(fetchSystemMetrics, 4000)

    return () => {
      window.removeEventListener('hashchange', handleHashChange)
      clearInterval(metricsTimer)
    }
  }, [handleHashChange, loadData, fetchSystemMetrics])

  const navigateToView = (view: ViewMode, id?: string) => {
    if (view === 'execution-detail' && id) {
      window.location.hash = `execution/${id}`
    } else if (view === 'workflow-detail' && id) {
      window.location.hash = `workflows/${id}`
    } else if (view === 'history') {
      window.location.hash = 'history'
    } else if (view === 'workflows') {
      window.location.hash = 'workflows'
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

      {/* Main Content Workspace */}
      <div className="flex-1 flex flex-col h-screen overflow-hidden bg-[#F4F6F9]">
        {/* Workspace Body */}
        <main className="flex-1 overflow-y-auto px-8 py-6 space-y-5">
          {currentView === 'dashboard' && (
            <DashboardView
              workflows={workflows}
              executions={executions}
              systemMetrics={systemMetrics}
              isLoading={isLoading}
              onRefresh={() => {
                loadData()
                fetchSystemMetrics()
              }}
              onNavigate={navigateToView}
            />
          )}

          {currentView === 'workflows' && (
            <WorkflowsView
              workflows={workflows}
              isLoading={isLoading}
              onRefresh={() => loadData()}
              onSelectWorkflow={(id) => navigateToView('workflow-detail', id)}
            />
          )}

          {currentView === 'workflow-detail' && selectedWorkflowId && (
            <WorkflowDetailView
              workflowId={selectedWorkflowId}
              onBack={() => navigateToView('workflows')}
              onNavigateToExecution={(execId) => navigateToView('execution-detail', execId)}
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

        {/* Slim System Status Footer Bar (RAM & CPU) */}
        <footer className="h-7 bg-white border-t border-slate-300/70 px-6 flex items-center justify-between text-[11px] font-mono text-slate-500 select-none flex-shrink-0">
          <div className="flex items-center space-x-3.5">
            <span className="flex items-center gap-1">
              <Cpu className="w-3.5 h-3.5 text-slate-400" />
              <span>CPU:</span>
              <strong className="text-slate-800 font-semibold">
                {systemMetrics?.cpu_percent?.toFixed(1) ?? '—'}%
              </strong>
            </span>
            <span className="text-slate-300">|</span>
            <span className="flex items-center gap-1">
              <HardDrive className="w-3.5 h-3.5 text-slate-400" />
              <span>RAM:</span>
              <strong className="text-slate-800 font-semibold">
                {systemMetrics?.go_alloc_mb?.toFixed(1) ?? '—'} MB
              </strong>
              {systemMetrics && (
                <span className="text-slate-400">
                  ({(systemMetrics.memory_used_mb / 1024).toFixed(1)}/{(systemMetrics.memory_total_mb / 1024).toFixed(0)} GB • {systemMetrics.memory_percent.toFixed(0)}%)
                </span>
              )}
            </span>
          </div>

          <div className="hidden sm:flex items-center space-x-3 text-slate-400 text-[10px]">
            <span>Go 1.26.1</span>
            <span>•</span>
            <span>Goroutines: <strong className="text-slate-600">{systemMetrics?.goroutines ?? '—'}</strong></span>
          </div>
        </footer>
      </div>
    </div>
  )
}

export default App
