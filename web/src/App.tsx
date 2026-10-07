import React, { useState, useEffect, useCallback } from 'react'
import { Cpu, HardDrive } from 'lucide-react'
import type { Workflow, Execution, ViewMode, SystemMetrics } from './types/relay'
import { Sidebar } from './components/layout/Sidebar'
import { DashboardView } from './components/dashboard/DashboardView'
import { WorkflowsView } from './components/workflows/WorkflowsView'
import { HistoryView } from './components/history/HistoryView'
import { ExecutionDetailView } from './components/execution-detail/ExecutionDetailView'
import { WorkflowDetailView } from './components/workflows/WorkflowDetailView'

const initialSystemMetrics: SystemMetrics = {
  cpu_percent: 0,
  memory_used_mb: 0,
  memory_total_mb: 0,
  memory_percent: 0,
  go_alloc_mb: 0,
  go_sys_mb: 0,
  goroutines: 0,
  num_cpu: 0,
  uptime_seconds: 0,
}

export const App: React.FC = () => {
  const [workflows, setWorkflows] = useState<Workflow[]>([])
  const [executions, setExecutions] = useState<Execution[]>([])
  const [systemMetrics, setSystemMetrics] = useState<SystemMetrics>(initialSystemMetrics)
  const [isLoading, setIsLoading] = useState<boolean>(false)
  const [currentView, setCurrentView] = useState<ViewMode>('dashboard')
  const [selectedExecutionId, setSelectedExecutionId] = useState<string | null>(null)
  const [selectedWorkflowId, setSelectedWorkflowId] = useState<string | null>(null)
  const [isSidebarCollapsed, setIsSidebarCollapsed] = useState<boolean>(false)

  // Clear any legacy mock mode preference
  useEffect(() => {
    localStorage.removeItem('relay_mock_mode')
  }, [])

  // Fetch real-time system metrics (CPU, RAM, Goroutines)
  const fetchSystemMetrics = useCallback(async () => {
    try {
      const res = await fetch('/system/metrics').catch(() => null)
      if (res && res.ok) {
        const data: SystemMetrics = await res.json().catch(() => null)
        if (data) {
          setSystemMetrics(data)
        }
      }
    } catch (err) {
      console.warn('System metrics fetch error:', err)
    }
  }, [])

  // Fetch live workflows and executions
  const loadData = useCallback(async () => {
    setIsLoading(true)

    try {
      const [execRes, wfRes] = await Promise.all([
        fetch('/executions').catch(() => null),
        fetch('/workflows').catch(() => null),
      ])

      const execData = execRes && execRes.ok ? await execRes.json().catch(() => []) : []
      const wfData = wfRes && wfRes.ok ? await wfRes.json().catch(() => []) : []

      setExecutions(Array.isArray(execData) ? execData : [])
      setWorkflows(Array.isArray(wfData) ? wfData : [])
    } catch (err) {
      console.error('Failed to load workflows or executions from server:', err)
      setExecutions([])
      setWorkflows([])
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
    <div className="text-neutral-200 bg-[#0c0c0c] flex h-screen overflow-hidden antialiased">
      {/* Refactored Clean Left Sidebar */}
      <Sidebar
        currentView={currentView}
        onSelectView={(v) => navigateToView(v)}
        workflowCount={workflows.length}
        executionCount={executions.length}
        isCollapsed={isSidebarCollapsed}
        onToggleCollapse={() => setIsSidebarCollapsed(!isSidebarCollapsed)}
      />

      {/* Main Content Workspace */}
      <div className="flex-1 flex flex-col h-screen overflow-hidden bg-[#0c0c0c]">
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
        <footer className="h-7 bg-[#111111] border-t border-neutral-800 px-6 flex items-center justify-between text-[11px] font-mono text-neutral-400 select-none flex-shrink-0">
          <div className="flex items-center space-x-3.5">
            <span className="flex items-center gap-1">
              <Cpu className="w-3.5 h-3.5 text-neutral-500" />
              <span>CPU:</span>
              <strong className="text-neutral-200 font-semibold">
                {systemMetrics?.cpu_percent?.toFixed(1) ?? '—'}%
              </strong>
            </span>
            <span className="text-neutral-700">|</span>
            <span className="flex items-center gap-1">
              <HardDrive className="w-3.5 h-3.5 text-neutral-500" />
              <span>RAM:</span>
              <strong className="text-neutral-200 font-semibold">
                {systemMetrics?.go_alloc_mb?.toFixed(1) ?? '—'} MB
              </strong>
              {systemMetrics && (
                <span className="text-neutral-500">
                  ({(systemMetrics.memory_used_mb / 1024).toFixed(1)}/{(systemMetrics.memory_total_mb / 1024).toFixed(0)} GB • {systemMetrics.memory_percent.toFixed(0)}%)
                </span>
              )}
            </span>
          </div>

          <div className="hidden sm:flex items-center space-x-3 text-neutral-500 text-[10px]">
            <span>Go 1.26.1</span>
            <span>•</span>
            <span>Goroutines: <strong className="text-neutral-400">{systemMetrics?.goroutines ?? '—'}</strong></span>
          </div>
        </footer>
      </div>
    </div>
  )
}

export default App
