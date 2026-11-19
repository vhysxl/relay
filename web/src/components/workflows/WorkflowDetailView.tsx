import React, { useState, useEffect, useMemo, useRef, useCallback } from 'react'
import {
  ArrowLeft,
  RotateCw,
  ShieldAlert,
  ChevronDown,
  ChevronRight,
  ExternalLink,
  Loader2,
  Workflow as WorkflowIcon,
  ListOrdered,
  FileCode,
  GitBranch,
  History,
  AlertCircle,
  Plus,
  Minus,
  Edit3,
  Code2,
  Eye,
  EyeOff,
  CheckCircle2,
} from 'lucide-react'
import type {
  Workflow,
  Execution,
  WorkflowSummary,
  WorkflowVersionChangesResponse,
} from '../../types/relay'
import { StatusBadge } from '../common/StatusBadge'
import { formatTimestamp, formatJSON } from '../../utils/format'
import { parseLosslessJSON } from '../../utils/json'
import { sortStepsTopologically } from '../../utils/dag'
import { WorkflowDagCanvas } from './WorkflowDagCanvas'

interface WorkflowDetailViewProps {
  workflowName: string
  onBack: () => void
  onNavigateToExecution?: (executionId: string) => void
}

export const WorkflowDetailView: React.FC<WorkflowDetailViewProps> = ({
  workflowName,
  onBack,
  onNavigateToExecution,
}) => {
  const targetName = workflowName

  // Latest workflow definition and executions
  const [workflow, setWorkflow] = useState<Workflow | null>(null)
  const [recentExecutions, setRecentExecutions] = useState<Execution[]>([])
  const [hideSuccessfulExecutions, setHideSuccessfulExecutions] = useState<boolean>(true)
  const [isLoading, setIsLoading] = useState<boolean>(true)
  const [error, setError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'canvas' | 'list' | 'raw' | 'history'>('canvas')
  const [expandedSteps, setExpandedSteps] = useState<Record<string, boolean>>({})

  const completedExecutionsCount = useMemo(() => {
    return recentExecutions.filter((e) => e.status === 'COMPLETED').length
  }, [recentExecutions])

  const displayedExecutions = useMemo(() => {
    if (hideSuccessfulExecutions) {
      return recentExecutions.filter((e) => e.status !== 'COMPLETED')
    }
    return recentExecutions
  }, [recentExecutions, hideSuccessfulExecutions])

  // Version History state
  const [versions, setVersions] = useState<WorkflowSummary[]>([])
  const [versionsLoading, setVersionsLoading] = useState<boolean>(false)
  const [versionsError, setVersionsError] = useState<string | null>(null)
  const [hasMoreVersions, setHasMoreVersions] = useState<boolean>(false)
  const [selectedVersion, setSelectedVersion] = useState<number | null>(null)

  // Keep a ref to versions for stable pagination without effect re-triggers
  const versionsRef = useRef<WorkflowSummary[]>([])
  versionsRef.current = versions

  // Selected Version details & diff
  const [versionDetailsLoading, setVersionDetailsLoading] = useState<boolean>(false)
  const [versionDetailsError, setVersionDetailsError] = useState<string | null>(null)
  const [versionDefinition, setVersionDefinition] = useState<Workflow | null>(null)
  const [versionChanges, setVersionChanges] = useState<WorkflowVersionChangesResponse | null>(null)
  const [showRawVersion, setShowRawVersion] = useState<boolean>(false)
  const [expandedChangePaths, setExpandedChangePaths] = useState<Record<string, boolean>>({})

  // Request sequencing to discard stale responses
  const versionsReqIdRef = useRef(0)
  const versionDetailReqIdRef = useRef(0)

  const orderedSteps = useMemo(() => {
    return sortStepsTopologically(workflow?.steps || [])
  }, [workflow?.steps])

  const toggleStepExpand = (stepId: string) => {
    setExpandedSteps((prev) => ({ ...prev, [stepId]: !prev[stepId] }))
  }

  const toggleChangeExpand = (path: string) => {
    setExpandedChangePaths((prev) => ({ ...prev, [path]: !prev[path] }))
  }

  // Render a before/after value explicitly preserving null, false, 0, and large numbers
  const renderChangeValue = (val: any) => {
    if (val === undefined) return ''
    if (val === null) return 'null'
    if (typeof val === 'boolean') return val ? 'true' : 'false'
    if (typeof val === 'number') return String(val)
    if (typeof val === 'string') return val
    return formatJSON(val)
  }

  // Fetch the latest workflow definition and recent executions
  const fetchWorkflow = useCallback(
    async (signal?: AbortSignal) => {
      if (!targetName) return
      setIsLoading(true)
      setError(null)

      try {
        const [wfRes, execRes] = await Promise.all([
          fetch(`/workflows/${encodeURIComponent(targetName)}`, { signal }),
          fetch('/executions', { signal }),
        ])

        if (!wfRes.ok) {
          throw new Error(`Workflow "${targetName}" not found`)
        }

        const [wfText, execText] = await Promise.all([
          wfRes.text(),
          execRes.ok ? execRes.text() : Promise.resolve('[]'),
        ])

        const data: Workflow = parseLosslessJSON<Workflow>(wfText)
        setWorkflow(data)

        const execs: Execution[] = parseLosslessJSON<Execution[]>(execText)
        setRecentExecutions(
          Array.isArray(execs)
            ? execs.filter(
                (e) =>
                  e.workflow_name === targetName ||
                  (data && (e.workflow_id === data.id || e.workflow_name === data.name))
              )
            : []
        )
      } catch (err: any) {
        if (err.name === 'AbortError' || signal?.aborted) return
        setError(err.message || 'Failed to load workflow')
      } finally {
        setIsLoading(false)
      }
    },
    [targetName]
  )

  // Fetch Version History list
  const fetchVersions = useCallback(
    async (reset: boolean = true, signal?: AbortSignal) => {
      if (!targetName) return
      const reqId = ++versionsReqIdRef.current
      setVersionsLoading(true)
      if (reset) {
        setVersionsError(null)
      }

      const offset = reset ? 0 : versionsRef.current.length
      try {
        const res = await fetch(
          `/workflows/${encodeURIComponent(targetName)}/versions?limit=50&offset=${offset}`,
          { signal }
        )
        if (reqId !== versionsReqIdRef.current) return

        if (!res.ok) {
          throw new Error('Gagal memuat riwayat versi')
        }

        const text = await res.text()
        if (reqId !== versionsReqIdRef.current) return

        const data: WorkflowSummary[] = parseLosslessJSON<WorkflowSummary[]>(text)
        if (reqId !== versionsReqIdRef.current) return

        const items = Array.isArray(data) ? data : []
        if (reset) {
          setVersions(items)
          setHasMoreVersions(items.length === 50)
          setSelectedVersion((prev) =>
            prev === null && items.length > 0 ? items[0].version : prev
          )
        } else {
          setVersions((prev) => [...prev, ...items])
          setHasMoreVersions(items.length === 50)
        }
      } catch (err: any) {
        if (err.name === 'AbortError' || signal?.aborted) return
        if (reqId !== versionsReqIdRef.current) return
        setVersionsError(err.message || 'Gagal memuat riwayat versi')
      } finally {
        if (reqId === versionsReqIdRef.current) {
          setVersionsLoading(false)
        }
      }
    },
    [targetName]
  )

  // Fetch specific version definition and changes
  const fetchVersionDetails = useCallback(
    async (versionNum: number, signal?: AbortSignal) => {
      if (!targetName) return
      const reqId = ++versionDetailReqIdRef.current
      setVersionDetailsLoading(true)
      setVersionDetailsError(null)
      setShowRawVersion(false)
      setExpandedChangePaths({})

      try {
        const [defRes, changesRes] = await Promise.all([
          fetch(`/workflows/${encodeURIComponent(targetName)}/versions/${versionNum}`, { signal }),
          fetch(`/workflows/${encodeURIComponent(targetName)}/versions/${versionNum}/changes`, {
            signal,
          }),
        ])

        if (reqId !== versionDetailReqIdRef.current) return

        if (!defRes.ok) {
          throw new Error(`Versi ${versionNum} tidak ditemukan`)
        }
        if (!changesRes.ok) {
          throw new Error(`Gagal memuat perubahan versi ${versionNum}`)
        }

        const [defText, changesText] = await Promise.all([defRes.text(), changesRes.text()])

        if (reqId !== versionDetailReqIdRef.current) return

        const defData: Workflow = parseLosslessJSON<Workflow>(defText)
        const changesData: WorkflowVersionChangesResponse =
          parseLosslessJSON<WorkflowVersionChangesResponse>(changesText)

        if (reqId !== versionDetailReqIdRef.current) return

        setVersionDefinition(defData)
        setVersionChanges(changesData)
      } catch (err: any) {
        if (err.name === 'AbortError' || signal?.aborted) return
        if (reqId !== versionDetailReqIdRef.current) return
        setVersionDetailsError(err.message || `Gagal memuat detail versi ${versionNum}`)
        setVersionDefinition(null)
        setVersionChanges(null)
      } finally {
        if (reqId === versionDetailReqIdRef.current) {
          setVersionDetailsLoading(false)
        }
      }
    },
    [targetName]
  )

  // Invalidate and reset all states whenever target workflow name changes
  useEffect(() => {
    versionsReqIdRef.current++
    versionDetailReqIdRef.current++

    setSelectedVersion(null)
    setVersions([])
    setVersionsLoading(false)
    setVersionsError(null)
    setHasMoreVersions(false)
    setVersionDefinition(null)
    setVersionChanges(null)
    setVersionDetailsLoading(false)
    setVersionDetailsError(null)
    setShowRawVersion(false)
    setExpandedChangePaths({})

    const controller = new AbortController()
    fetchWorkflow(controller.signal)

    return () => {
      controller.abort()
      versionsReqIdRef.current++
      versionDetailReqIdRef.current++
    }
  }, [targetName, fetchWorkflow])

  // When switching to History tab, load versions (independent of loading states to prevent request-abort loops)
  useEffect(() => {
    if (activeTab === 'history' && targetName) {
      const controller = new AbortController()
      fetchVersions(true, controller.signal)
      return () => {
        controller.abort()
      }
    }
  }, [activeTab, targetName, fetchVersions])

  // When a version is selected in History tab, load its details & changes
  useEffect(() => {
    if (activeTab === 'history' && targetName && selectedVersion !== null) {
      const controller = new AbortController()
      fetchVersionDetails(selectedVersion, controller.signal)
      return () => {
        controller.abort()
      }
    }
  }, [activeTab, targetName, selectedVersion, fetchVersionDetails])

  // Derived change statistics
  const changeStats = useMemo(() => {
    if (!versionChanges || !versionChanges.changes) {
      return { added: 0, removed: 0, modified: 0, context: 0 }
    }
    const changes = versionChanges.changes
    let added = 0
    let removed = 0
    let modifiedStepIds = new Set<string>()
    let context = 0

    for (const c of changes) {
      if (c.path.startsWith('/context')) {
        context++
      } else if (c.path.startsWith('/steps/')) {
        const parts = c.path.split('/')
        // Whole step addition or removal has path "/steps/<id>"
        if (parts.length === 3) {
          if (c.kind === 'added') added++
          else if (c.kind === 'removed') removed++
          else modifiedStepIds.add(parts[2])
        } else if (parts.length > 3) {
          modifiedStepIds.add(parts[2])
        }
      }
    }

    return {
      added,
      removed,
      modified: modifiedStepIds.size,
      context,
    }
  }, [versionChanges])

  if (isLoading && !workflow) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-12 text-center space-y-3 shadow-xs">
        <Loader2 className="animate-spin h-7 w-7 text-neutral-400 mx-auto" />
        <p className="text-sm font-semibold text-neutral-100">Loading Workflow Definition...</p>
        <p className="text-xs font-mono text-neutral-500">{targetName}</p>
      </div>
    )
  }

  if (error || !workflow) {
    return (
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-rose-900/60 p-8 text-center space-y-3 shadow-xs">
        <h3 className="text-base font-bold text-neutral-100">Workflow Not Found</h3>
        <p className="text-xs text-rose-400">{error || 'Could not find workflow'}</p>
        <div className="pt-2">
          <button
            onClick={onBack}
            className="px-3.5 py-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-100 border border-neutral-700 rounded-lg text-xs font-medium transition cursor-pointer"
          >
            Back to Workflows
          </button>
        </div>
      </div>
    )
  }

  const steps = workflow.steps || []
  const hasCompensation = steps.some((s) => s.compensation)

  return (
    <div className="space-y-4">
      {/* Top Header Block matching standard pages */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-2 border-b border-neutral-800">
        <div className="flex items-center space-x-3">
          <button
            onClick={onBack}
            className="p-1.5 rounded-lg border border-neutral-700/80 bg-[#141414] hover:bg-neutral-800 text-neutral-400 hover:text-neutral-100 shadow-2xs transition cursor-pointer outline-none focus:outline-none focus:ring-0"
            title="Back to workflows"
          >
            <ArrowLeft className="w-4 h-4" />
          </button>

          <div className="space-y-0.5">
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold tracking-tight text-neutral-100">{workflow.name}</h1>
              <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-mono font-medium bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                v{workflow.version} (latest)
              </span>
            </div>
            <p className="text-xs font-mono text-neutral-400">ID: {workflow.id}</p>
          </div>
        </div>

        <div className="flex items-center space-x-2">
          <button
            onClick={() => {
              fetchWorkflow()
              if (activeTab === 'history') {
                fetchVersions(true)
              }
            }}
            disabled={isLoading || (activeTab === 'history' && versionsLoading)}
            className="px-3 py-1.5 bg-[#141414] border border-neutral-700/80 hover:bg-neutral-800 text-neutral-300 hover:text-neutral-100 text-xs font-medium rounded-lg shadow-2xs flex items-center space-x-1.5 transition disabled:opacity-50 cursor-pointer outline-none focus:outline-none focus:ring-0"
          >
            <RotateCw
              className={`w-3.5 h-3.5 text-neutral-400 ${
                isLoading || (activeTab === 'history' && versionsLoading) ? 'animate-spin' : ''
              }`}
            />
            <span>Refresh</span>
          </button>
        </div>
      </div>

      {/* Summary KPI Cards for Latest Definition */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Steps (Latest)
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-2xl font-bold tracking-tight text-neutral-100">{steps.length}</span>
            <span className="text-xs text-neutral-400 font-medium">DAG Nodes</span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Rollbacks
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-2xl font-bold tracking-tight text-neutral-100">
              {hasCompensation ? 'Enabled' : 'None'}
            </span>
            <span className="text-xs text-neutral-400 font-medium">
              {steps.filter((s) => s.compensation).length} steps
            </span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Executions
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-2xl font-bold tracking-tight text-neutral-100">
              {recentExecutions.length}
            </span>
            <span className="text-xs text-neutral-400 font-medium">Runs</span>
          </div>
        </div>

        <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 p-4 shadow-xs space-y-1">
          <span className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
            Created
          </span>
          <div className="flex items-baseline justify-between pt-1">
            <span className="text-xs font-mono font-bold text-neutral-300">
              {formatTimestamp(workflow.created_at)}
            </span>
          </div>
        </div>
      </div>

      {/* Tabs: DAG Canvas vs Step List vs Raw JSON vs Version History */}
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden">
        <div className="px-5 py-3 bg-[#181818] border-b border-neutral-800 flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <button
              id="workflow-tab-canvas"
              onClick={() => setActiveTab('canvas')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                activeTab === 'canvas'
                  ? 'bg-[#141414] text-neutral-100 shadow-2xs border border-neutral-700'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              <WorkflowIcon className="w-3.5 h-3.5" />
              <span>DAG Canvas</span>
            </button>
            <button
              id="workflow-tab-list"
              onClick={() => setActiveTab('list')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                activeTab === 'list'
                  ? 'bg-[#141414] text-neutral-100 shadow-2xs border border-neutral-700'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              <ListOrdered className="w-3.5 h-3.5" />
              <span>Step List ({steps.length})</span>
            </button>
            <button
              id="workflow-tab-raw"
              onClick={() => setActiveTab('raw')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                activeTab === 'raw'
                  ? 'bg-[#141414] text-neutral-100 shadow-2xs border border-neutral-700'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              <FileCode className="w-3.5 h-3.5" />
              <span>Raw Definition</span>
            </button>
            <button
              id="workflow-tab-history"
              onClick={() => setActiveTab('history')}
              className={`px-3 py-1 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5 ${
                activeTab === 'history'
                  ? 'bg-[#141414] text-neutral-100 shadow-2xs border border-neutral-700'
                  : 'text-neutral-400 hover:text-neutral-200'
              }`}
            >
              <History className="w-3.5 h-3.5" />
              <span>Version History</span>
            </button>
          </div>
        </div>

        {/* Tab 1: DAG Canvas (Always latest) */}
        {activeTab === 'canvas' && (
          <div className="p-3">
            <WorkflowDagCanvas steps={steps} workflowName={workflow.name} />
          </div>
        )}

        {/* Tab 2: Step List (Always latest) */}
        {activeTab === 'list' && (
          <div className="divide-y divide-neutral-800/80">
            {orderedSteps.length === 0 ? (
              <div className="px-5 py-10 text-center text-neutral-500 text-xs">
                No steps defined in this workflow.
              </div>
            ) : (
              orderedSteps.map((step, idx) => {
                const isExpanded = expandedSteps[step.id]
                return (
                  <div key={step.id || idx} className="p-4 space-y-3">
                    <div
                      onClick={() => toggleStepExpand(step.id)}
                      className="flex items-center justify-between cursor-pointer group"
                    >
                      <div className="flex items-center space-x-3">
                        <span className="w-6 h-6 rounded-md bg-neutral-800 text-neutral-300 font-mono font-bold text-xs flex items-center justify-center">
                          {idx + 1}
                        </span>
                        <div>
                          <div className="flex items-center gap-2">
                            <span className="font-bold text-sm text-neutral-200 group-hover:underline">
                              {step.name}
                            </span>
                            <span className="font-mono text-xs text-neutral-500">({step.id})</span>
                          </div>
                        </div>
                      </div>

                      <div className="flex items-center space-x-2">
                        <span className="px-2 py-0.5 rounded text-xs font-mono font-medium uppercase bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                          {step.activity}
                        </span>
                        {step.when && (
                          <span
                            className="px-2 py-0.5 rounded text-xs font-mono font-medium bg-indigo-950/50 text-indigo-400 border border-indigo-800/60 flex items-center gap-1"
                            title="Conditional step (when)"
                          >
                            <GitBranch className="w-3 h-3 text-indigo-400" />
                            <span>when</span>
                          </span>
                        )}
                        {step.compensation && (
                          <span className="px-2 py-0.5 rounded text-xs font-medium bg-amber-950/50 text-amber-400 border border-amber-800/60">
                            Rollback
                          </span>
                        )}
                        {step.retry?.max_attempts && (
                          <span className="px-2 py-0.5 rounded text-xs font-mono font-medium bg-neutral-800/80 text-neutral-400 border border-neutral-700/60">
                            {step.retry.max_attempts}x
                          </span>
                        )}
                        <span className="text-neutral-500 pl-1">
                          {isExpanded ? (
                            <ChevronDown className="w-4 h-4" />
                          ) : (
                            <ChevronRight className="w-4 h-4" />
                          )}
                        </span>
                      </div>
                    </div>

                    {/* Expanded Step Details */}
                    {isExpanded && (
                      <div className="mt-2 pl-9 space-y-3 pt-2 border-t border-neutral-800 text-xs">
                        <p className="text-neutral-400">
                          Queue: <span className="font-mono text-neutral-200">{step.queue}</span>
                        </p>
                        {step.depends_on && step.depends_on.length > 0 && (
                          <div>
                            <span className="text-neutral-400 font-medium">Depends on: </span>
                            <span className="font-mono text-neutral-200">
                              {step.depends_on.join(', ')}
                            </span>
                          </div>
                        )}

                        {step.when && (
                          <div className="space-y-1">
                            <div className="flex items-center gap-1.5 text-indigo-400 font-medium">
                              <GitBranch className="w-3.5 h-3.5" />
                              <span>Condition (When):</span>
                            </div>
                            <pre className="p-3 bg-[#0c0c0c] border border-indigo-950/60 text-indigo-300 rounded-md font-mono text-[11px] overflow-x-auto">
                              {formatJSON(step.when)}
                            </pre>
                          </div>
                        )}

                        {step.input !== undefined && (
                          <div className="space-y-1">
                            <span className="text-neutral-400 font-medium">Input Template:</span>
                            <pre className="p-3 bg-[#0c0c0c] border border-neutral-800 rounded-md font-mono text-neutral-300 text-[11px] overflow-x-auto">
                              {formatJSON(step.input)}
                            </pre>
                          </div>
                        )}

                        {step.compensation && (
                          <div className="p-3 bg-neutral-900 border border-neutral-700/80 rounded-md space-y-1.5">
                            <div className="flex items-center gap-1.5 font-semibold text-amber-400">
                              <ShieldAlert className="w-3.5 h-3.5 text-amber-400" />
                              <span>Rollback: {step.compensation.activity}</span>
                            </div>
                            {step.compensation.input !== undefined && (
                              <pre className="p-2 bg-[#0c0c0c] border border-neutral-800 rounded font-mono text-neutral-300 text-[11px] overflow-x-auto">
                                {formatJSON(step.compensation.input)}
                              </pre>
                            )}
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                )
              })
            )}
          </div>
        )}

        {/* Tab 3: Raw Definition (Always latest) */}
        {activeTab === 'raw' && (
          <div className="p-4">
            <pre className="p-4 bg-[#0c0c0c] border border-neutral-800 text-emerald-400 font-mono text-xs rounded-lg overflow-x-auto">
              {formatJSON(workflow)}
            </pre>
          </div>
        )}

        {/* Tab 4: Version History (Inspection only) */}
        {activeTab === 'history' && (
          <div className="grid grid-cols-1 md:grid-cols-12 divide-y md:divide-y-0 md:divide-x divide-neutral-800/80 min-h-[420px]">
            {/* Left Column: Version List */}
            <div className="md:col-span-4 lg:col-span-3 flex flex-col bg-[#141414]">
              <div className="px-4 py-3 bg-[#181818] border-b border-neutral-800 flex items-center justify-between text-xs text-neutral-400">
                <span className="font-semibold uppercase tracking-wider text-[11px]">
                  Daftar Versi
                </span>
                <span className="font-mono text-[11px]">
                  {versions.length} {versions.length === 1 ? 'versi' : 'versi'}
                </span>
              </div>

              {/* Version List Body */}
              <div className="flex-1 overflow-y-auto divide-y divide-neutral-800/60 max-h-[580px]">
                {versionsLoading && versions.length === 0 ? (
                  <div className="p-8 text-center text-xs text-neutral-500 space-y-2">
                    <Loader2 className="w-5 h-5 animate-spin mx-auto text-neutral-400" />
                    <p>Memuat riwayat versi...</p>
                  </div>
                ) : versionsError && versions.length === 0 ? (
                  <div className="p-6 text-center text-xs space-y-3">
                    <AlertCircle className="w-6 h-6 text-rose-400 mx-auto" />
                    <p className="text-rose-400">{versionsError}</p>
                    <button
                      onClick={() => fetchVersions(true)}
                      className="px-3 py-1 bg-neutral-800 hover:bg-neutral-700 text-neutral-200 border border-neutral-700 rounded text-xs transition cursor-pointer"
                    >
                      Coba Lagi
                    </button>
                  </div>
                ) : versions.length === 0 ? (
                  <div className="p-8 text-center text-xs text-neutral-500">
                    Tidak ada riwayat versi untuk workflow ini.
                  </div>
                ) : (
                  <>
                    {versions.map((v) => {
                      const isSelected = selectedVersion === v.version
                      const isLatest = v.version === workflow.version
                      return (
                        <div
                          key={v.version}
                          onClick={() => setSelectedVersion(v.version)}
                          className={`p-3.5 cursor-pointer transition flex flex-col gap-1 text-xs ${
                            isSelected
                              ? 'bg-neutral-800/90 border-l-2 border-emerald-500'
                              : 'hover:bg-neutral-800/40'
                          }`}
                        >
                          <div className="flex items-center justify-between">
                            <div className="flex items-center gap-1.5">
                              <span
                                className={`font-mono font-bold text-xs ${
                                  isSelected ? 'text-white' : 'text-neutral-200'
                                }`}
                              >
                                v{v.version}
                              </span>
                              {isLatest && (
                                <span className="px-1.5 py-0.2 rounded text-[10px] font-medium bg-emerald-950/60 text-emerald-400 border border-emerald-800/60">
                                  latest
                                </span>
                              )}
                            </div>
                            <span className="font-mono text-[11px] px-1.5 py-0.5 rounded bg-neutral-800 text-neutral-300 border border-neutral-700/60">
                              {v.step_count} {v.step_count === 1 ? 'step' : 'steps'}
                            </span>
                          </div>
                          <div className="text-[11px] font-mono text-neutral-400">
                            {formatTimestamp(v.created_at)}
                          </div>
                        </div>
                      )
                    })}

                    {/* Pagination: "Muat lebih" button */}
                    {hasMoreVersions && (
                      <div className="p-3 text-center">
                        <button
                          onClick={() => fetchVersions(false)}
                          disabled={versionsLoading}
                          className="w-full py-1.5 px-3 bg-[#181818] hover:bg-neutral-800 text-neutral-300 border border-neutral-700/80 rounded text-xs font-medium transition cursor-pointer disabled:opacity-50 flex items-center justify-center gap-1.5"
                        >
                          {versionsLoading ? (
                            <>
                              <Loader2 className="w-3.5 h-3.5 animate-spin text-neutral-400" />
                              <span>Memuat...</span>
                            </>
                          ) : (
                            <span>Muat lebih (50 berikutnya)</span>
                          )}
                        </button>
                      </div>
                    )}
                  </>
                )}
              </div>
            </div>

            {/* Right Column: Selected Version Inspector & Diff */}
            <div className="md:col-span-8 lg:col-span-9 p-5 flex flex-col space-y-4 bg-[#141414]">
              {versionDetailsLoading ? (
                <div className="p-12 text-center text-xs text-neutral-500 space-y-2">
                  <Loader2 className="w-6 h-6 animate-spin mx-auto text-neutral-400" />
                  <p>Memuat perubahan versi {selectedVersion}...</p>
                </div>
              ) : versionDetailsError ? (
                <div className="p-8 text-center text-xs space-y-3 bg-[#181818] border border-rose-900/60 rounded-lg">
                  <AlertCircle className="w-6 h-6 text-rose-400 mx-auto" />
                  <p className="text-rose-400">{versionDetailsError}</p>
                  <button
                    onClick={() => selectedVersion !== null && fetchVersionDetails(selectedVersion)}
                    className="px-3 py-1.5 bg-neutral-800 hover:bg-neutral-700 text-neutral-200 border border-neutral-700 rounded text-xs transition cursor-pointer"
                  >
                    Coba Lagi
                  </button>
                </div>
              ) : selectedVersion === null || !versionDefinition ? (
                <div className="p-12 text-center text-xs text-neutral-500">
                  Pilih versi dari daftar di sebelah kiri untuk melihat rincian perubahan.
                </div>
              ) : (
                <>
                  {/* Version Header Block */}
                  <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 pb-3 border-b border-neutral-800">
                    <div className="space-y-1">
                      <div className="flex items-center gap-2">
                        <h2 className="text-base font-bold text-neutral-100">
                          Detail Versi {versionDefinition.version}
                        </h2>
                        {versionChanges?.from_version === null && (
                          <span className="px-2 py-0.5 rounded text-xs font-semibold bg-emerald-950/60 text-emerald-400 border border-emerald-800/60">
                            Versi pertama
                          </span>
                        )}
                        {versionChanges && versionChanges.from_version !== null && (
                          <span className="px-2 py-0.5 rounded text-xs font-mono bg-neutral-800 text-neutral-300 border border-neutral-700/60">
                            Perbandingan vs v{versionChanges.from_version}
                          </span>
                        )}
                      </div>
                      <p className="text-xs text-neutral-400 font-mono">
                        Dibuat: {formatTimestamp(versionDefinition.created_at)} •{' '}
                        {versionDefinition.steps?.length || 0} steps
                      </p>
                    </div>

                    <div className="flex items-center space-x-2">
                      <button
                        onClick={() => setShowRawVersion(!showRawVersion)}
                        className="px-3 py-1.5 bg-[#181818] hover:bg-neutral-800 text-neutral-300 hover:text-white border border-neutral-700/80 rounded-md text-xs font-medium transition cursor-pointer flex items-center gap-1.5"
                      >
                        <Code2 className="w-3.5 h-3.5 text-neutral-400" />
                        <span>{showRawVersion ? 'Sembunyikan JSON' : 'Lihat JSON Lengkap'}</span>
                      </button>
                    </div>
                  </div>

                  {/* If user toggled complete version raw definition */}
                  {showRawVersion && (
                    <div className="space-y-1.5">
                      <div className="text-xs font-semibold text-neutral-300">
                        Definisi JSON Lengkap (v{versionDefinition.version}):
                      </div>
                      <pre className="p-4 bg-[#0c0c0c] border border-neutral-800 text-emerald-400 font-mono text-xs rounded-lg overflow-x-auto max-h-96">
                        {formatJSON(versionDefinition)}
                      </pre>
                    </div>
                  )}

                  {/* First Version Callout */}
                  {versionChanges?.from_version === null ? (
                    <div className="p-6 bg-[#181818] border border-neutral-800 rounded-lg text-center space-y-1.5">
                      <div className="text-sm font-semibold text-neutral-200">Versi Pertama</div>
                      <p className="text-xs text-neutral-400">
                        Ini adalah versi awal workflow. Tidak ada versi sebelumnya yang tersedia
                        untuk dibandingkan.
                      </p>
                    </div>
                  ) : versionChanges ? (
                    <>
                      {/* Summary Badges: Steps Added, Removed, Modified, and Context Changes */}
                      <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                        <div className="p-3 bg-[#181818] border border-neutral-800 rounded-lg space-y-1">
                          <span className="text-[10px] uppercase font-semibold text-neutral-400">
                            Step Ditambah
                          </span>
                          <div className="text-lg font-bold text-emerald-400">
                            +{changeStats.added}
                          </div>
                        </div>
                        <div className="p-3 bg-[#181818] border border-neutral-800 rounded-lg space-y-1">
                          <span className="text-[10px] uppercase font-semibold text-neutral-400">
                            Step Dihapus
                          </span>
                          <div className="text-lg font-bold text-rose-400">
                            -{changeStats.removed}
                          </div>
                        </div>
                        <div className="p-3 bg-[#181818] border border-neutral-800 rounded-lg space-y-1">
                          <span className="text-[10px] uppercase font-semibold text-neutral-400">
                            Step Diubah
                          </span>
                          <div className="text-lg font-bold text-amber-400">
                            ~{changeStats.modified}
                          </div>
                        </div>
                        <div className="p-3 bg-[#181818] border border-neutral-800 rounded-lg space-y-1">
                          <span className="text-[10px] uppercase font-semibold text-neutral-400">
                            Perubahan Context
                          </span>
                          <div className="text-lg font-bold text-sky-400">
                            {changeStats.context}
                          </div>
                        </div>
                      </div>

                      {/* Changes List */}
                      <div className="space-y-2 pt-2">
                        <div className="text-xs font-semibold text-neutral-300">
                          Rincian Perubahan ({versionChanges.changes?.length || 0}):
                        </div>

                        {!versionChanges.changes || versionChanges.changes.length === 0 ? (
                          <div className="p-6 bg-[#181818] border border-neutral-800 rounded-lg text-center text-xs text-neutral-400">
                            Tidak ada perbedaan definisi dari versi sebelumnya.
                          </div>
                        ) : (
                          <div className="divide-y divide-neutral-800/80 border border-neutral-800 rounded-lg overflow-hidden bg-[#181818]">
                            {versionChanges.changes.map((change, idx) => {
                              const isExpanded = expandedChangePaths[change.path]
                              const hasBefore = change.before !== undefined
                              const hasAfter = change.after !== undefined
                              return (
                                <div key={change.path || idx} className="p-3 space-y-2">
                                  <div
                                    onClick={() => toggleChangeExpand(change.path)}
                                    className="flex items-center justify-between cursor-pointer group"
                                  >
                                    <div className="flex items-center space-x-2.5 overflow-hidden">
                                      {change.kind === 'added' && (
                                        <span className="px-2 py-0.5 rounded text-[11px] font-mono font-bold bg-emerald-950/60 text-emerald-400 border border-emerald-800/60 flex items-center gap-1">
                                          <Plus className="w-3 h-3" />
                                          <span>added</span>
                                        </span>
                                      )}
                                      {change.kind === 'removed' && (
                                        <span className="px-2 py-0.5 rounded text-[11px] font-mono font-bold bg-rose-950/60 text-rose-400 border border-rose-800/60 flex items-center gap-1">
                                          <Minus className="w-3 h-3" />
                                          <span>removed</span>
                                        </span>
                                      )}
                                      {change.kind === 'modified' && (
                                        <span className="px-2 py-0.5 rounded text-[11px] font-mono font-bold bg-amber-950/60 text-amber-400 border border-amber-800/60 flex items-center gap-1">
                                          <Edit3 className="w-3 h-3" />
                                          <span>modified</span>
                                        </span>
                                      )}

                                      <span className="font-mono text-xs text-neutral-200 group-hover:underline truncate">
                                        {change.path}
                                      </span>
                                    </div>

                                    <div className="flex items-center space-x-2 text-neutral-400">
                                      <span className="text-[11px]">
                                        {isExpanded ? 'Tutup' : 'Lihat'}
                                      </span>
                                      {isExpanded ? (
                                        <ChevronDown className="w-4 h-4" />
                                      ) : (
                                        <ChevronRight className="w-4 h-4" />
                                      )}
                                    </div>
                                  </div>

                                  {/* Inspectable Before / After Details */}
                                  {isExpanded && (
                                    <div className="mt-2 pt-2 border-t border-neutral-800/80 grid grid-cols-1 md:grid-cols-2 gap-3 text-xs">
                                      {hasBefore && (
                                        <div className="space-y-1">
                                          <span className="text-[11px] font-semibold text-rose-400">
                                            Sebelum (v{versionChanges.from_version}):
                                          </span>
                                          <pre className="p-2.5 bg-[#0e0e0e] border border-rose-950/60 text-rose-300 rounded font-mono text-[11px] overflow-x-auto max-h-60">
                                            {renderChangeValue(change.before)}
                                          </pre>
                                        </div>
                                      )}
                                      {hasAfter && (
                                        <div className="space-y-1">
                                          <span className="text-[11px] font-semibold text-emerald-400">
                                            Sesudah (v{versionChanges.to_version}):
                                          </span>
                                          <pre className="p-2.5 bg-[#0e0e0e] border border-emerald-950/60 text-emerald-300 rounded font-mono text-[11px] overflow-x-auto max-h-60">
                                            {renderChangeValue(change.after)}
                                          </pre>
                                        </div>
                                      )}
                                    </div>
                                  )}
                                </div>
                              )
                            })}
                          </div>
                        )}
                      </div>
                    </>
                  ) : null}
                </>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Recent Executions Table for this Workflow */}
      <div className="bg-[#141414] rounded-lg border-[1.5px] border-neutral-800 shadow-xs overflow-hidden">
        <div className="px-5 py-3 bg-[#181818] border-b border-neutral-800 flex items-center justify-between">
          <div className="flex items-center space-x-2">
            <h2 className="text-[11px] font-semibold text-neutral-400 uppercase tracking-wider">
              Recent Executions
            </h2>
            <span className="text-neutral-500 font-mono text-[11px]">
              ({displayedExecutions.length} of {recentExecutions.length})
            </span>
          </div>

          {recentExecutions.length > 0 && (
            <button
              type="button"
              onClick={() => setHideSuccessfulExecutions(!hideSuccessfulExecutions)}
              className={`px-2.5 py-1 rounded-md border text-xs font-medium flex items-center space-x-1.5 transition cursor-pointer outline-none ${
                hideSuccessfulExecutions
                  ? 'bg-amber-950/40 border-amber-800/60 text-amber-300 hover:bg-amber-900/50'
                  : 'bg-neutral-800 border-neutral-700/80 text-neutral-400 hover:text-neutral-200 hover:bg-neutral-700'
              }`}
              title={
                hideSuccessfulExecutions
                  ? 'Click to show all executions'
                  : 'Click to hide successful executions'
              }
            >
              {hideSuccessfulExecutions ? (
                <EyeOff className="w-3.5 h-3.5 text-amber-400" />
              ) : (
                <Eye className="w-3.5 h-3.5 text-neutral-400" />
              )}
              <span>{hideSuccessfulExecutions ? 'Hide Successful' : 'Show All'}</span>
              {completedExecutionsCount > 0 && (
                <span className="px-1.5 py-0.5 rounded text-[10px] font-mono bg-amber-900/60 text-amber-200">
                  {completedExecutionsCount} hidden
                </span>
              )}
            </button>
          )}
        </div>

        <div className="divide-y divide-neutral-800/80">
          {displayedExecutions.length === 0 ? (
            <div className="px-5 py-8 text-center text-neutral-400 text-xs">
              {recentExecutions.length > 0 ? (
                <div className="space-y-1">
                  <CheckCircle2 className="w-5 h-5 text-emerald-400 mx-auto mb-1" />
                  <p className="font-semibold text-neutral-200">All recent executions succeeded</p>
                  <p className="text-[11px] text-neutral-500">
                    {completedExecutionsCount} successful execution
                    {completedExecutionsCount > 1 ? 's are' : ' is'} hidden.
                  </p>
                  <button
                    type="button"
                    onClick={() => setHideSuccessfulExecutions(false)}
                    className="mt-2 px-3 py-1 bg-neutral-800 hover:bg-neutral-700 text-neutral-200 rounded border border-neutral-700 text-xs transition cursor-pointer"
                  >
                    Show All Executions
                  </button>
                </div>
              ) : (
                <span>No executions recorded for this workflow yet.</span>
              )}
            </div>
          ) : (
            displayedExecutions.map((exec) => (
              <div
                key={exec.id}
                onClick={() => onNavigateToExecution && onNavigateToExecution(exec.id)}
                className="px-5 py-3 hover:bg-neutral-800/40 transition-colors group cursor-pointer flex items-center justify-between text-xs"
              >
                <div className="space-y-0.5">
                  <div className="flex items-center gap-2">
                    <span className="font-mono font-bold text-neutral-200 group-hover:underline">
                      #{exec.id.slice(0, 10)}
                    </span>
                    <span className="font-mono text-neutral-500">
                      {formatTimestamp(exec.created_at)}
                    </span>
                  </div>
                </div>
                <div className="flex items-center space-x-3">
                  <StatusBadge status={exec.status} size="sm" />
                  <ExternalLink className="w-3.5 h-3.5 text-neutral-500 group-hover:text-neutral-300 transition" />
                </div>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}
