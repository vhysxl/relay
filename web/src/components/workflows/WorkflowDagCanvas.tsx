import React, { useMemo, useState, useCallback, useEffect } from 'react'
import {
  ReactFlow,
  Controls,
  Background,
  BackgroundVariant,
  MiniMap,
  Handle,
  Position,
  MarkerType,
  type Node,
  type Edge,
  useNodesState,
  useEdgesState,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'
import dagre from '@dagrejs/dagre'
import {
  ShieldAlert,
  Copy,
  Check,
  X,
  Layers,
} from 'lucide-react'
import type { Step } from '../../types/relay'
import { formatJSON } from '../../utils/format'

interface WorkflowDagCanvasProps {
  steps: Step[]
  workflowName: string
}

// 1. Custom Node Component for Workflow Step
interface StepNodeData {
  step: Step
  isSelected: boolean
}

const StepCustomNode: React.FC<{ data: StepNodeData }> = ({ data }) => {
  const { step, isSelected } = data
  const hasRollback = Boolean(step.compensation)

  return (
    <div
      className={`w-[240px] bg-white rounded-lg border-[1.5px] p-3 shadow-xs transition-all select-none cursor-pointer flex flex-col justify-between ${
        isSelected
          ? 'border-indigo-600 ring-2 ring-indigo-500/30 shadow-md'
          : 'border-slate-300/80 hover:border-slate-400/90'
      }`}
    >
      {/* Target Handle (Left) */}
      <Handle
        type="target"
        position={Position.Left}
        className="!w-2.5 !h-2.5 !bg-slate-400 !border-2 !border-white hover:!bg-indigo-500"
      />

      {/* Top Header */}
      <div className="flex items-center justify-between gap-1 mb-1.5">
        <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium uppercase bg-slate-100 text-slate-700 border border-slate-200/80">
          {step.type}
        </span>

        <div className="flex items-center space-x-1">
          {step.retry?.max_attempts && (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium text-slate-500 bg-slate-50 border border-slate-200/60">
              {step.retry.max_attempts}x
            </span>
          )}
          {hasRollback && (
            <span
              className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-50 text-amber-800 border border-amber-200/80 flex items-center gap-0.5"
              title="Rollback handler defined"
            >
              <ShieldAlert className="w-2.5 h-2.5 text-amber-600" />
              <span>Rollback</span>
            </span>
          )}
        </div>
      </div>

      {/* Center Details */}
      <div className="space-y-0.5">
        <p className="font-bold text-xs text-slate-900 truncate" title={step.name}>
          {step.name}
        </p>
        <p className="font-mono text-[10px] text-slate-400 truncate" title={step.id}>
          {step.id}
        </p>
      </div>

      {/* Source Handle (Right) */}
      <Handle
        type="source"
        position={Position.Right}
        className="!w-2.5 !h-2.5 !bg-slate-400 !border-2 !border-white hover:!bg-indigo-500"
      />
    </div>
  )
}

const nodeTypes = {
  stepNode: StepCustomNode,
}

// 2. Dagre Automatic Layout Algorithm
const NODE_WIDTH = 240
const NODE_HEIGHT = 86

const getLayoutedElements = (
  nodes: Node[],
  edges: Edge[],
  direction = 'LR'
): { nodes: Node[]; edges: Edge[] } => {
  const dagreGraph = new dagre.graphlib.Graph()
  dagreGraph.setDefaultEdgeLabel(() => ({}))
  dagreGraph.setGraph({
    rankdir: direction,
    ranksep: 90,
    nodesep: 40,
    marginx: 40,
    marginy: 40,
  })

  nodes.forEach((node) => {
    dagreGraph.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT })
  })

  edges.forEach((edge) => {
    dagreGraph.setEdge(edge.source, edge.target)
  })

  dagre.layout(dagreGraph)

  const layoutedNodes = nodes.map((node) => {
    const nodeWithPosition = dagreGraph.node(node.id)
    return {
      ...node,
      targetPosition: Position.Left,
      sourcePosition: Position.Right,
      position: {
        x: nodeWithPosition.x - NODE_WIDTH / 2,
        y: nodeWithPosition.y - NODE_HEIGHT / 2,
      },
    }
  })

  return { nodes: layoutedNodes, edges }
}

export const WorkflowDagCanvas: React.FC<WorkflowDagCanvasProps> = ({
  steps,
  workflowName,
}) => {
  const [selectedStepId, setSelectedStepId] = useState<string | null>(null)
  const [copiedConfig, setCopiedConfig] = useState<boolean>(false)

  // Construct raw nodes and edges from Relay Step definitions
  const { initialNodes, initialEdges } = useMemo(() => {
    const rawNodes: Node[] = (steps || []).map((step) => ({
      id: step.id,
      type: 'stepNode',
      data: { step, isSelected: selectedStepId === step.id },
      position: { x: 0, y: 0 },
    }))

    const rawEdges: Edge[] = []
    ;(steps || []).forEach((step) => {
      if (step.depends_on && step.depends_on.length > 0) {
        step.depends_on.forEach((parentStepId) => {
          const isEdgeActive =
            selectedStepId === parentStepId || selectedStepId === step.id
          rawEdges.push({
            id: `${parentStepId}->${step.id}`,
            source: parentStepId,
            target: step.id,
            type: 'smoothstep',
            animated: isEdgeActive,
            style: {
              stroke: isEdgeActive ? '#6366F1' : '#94A3B8',
              strokeWidth: isEdgeActive ? 2.5 : 1.75,
            },
            markerEnd: {
              type: MarkerType.ArrowClosed,
              color: isEdgeActive ? '#6366F1' : '#94A3B8',
              width: 14,
              height: 14,
            },
          })
        })
      }
    })

    return { initialNodes: rawNodes, initialEdges: rawEdges }
  }, [steps, selectedStepId])

  // Run Dagre layout
  const { nodes: layoutedNodes, edges: layoutedEdges } = useMemo(() => {
    return getLayoutedElements(initialNodes, initialEdges, 'LR')
  }, [initialNodes, initialEdges])

  const [nodes, setNodes, onNodesChange] = useNodesState(layoutedNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(layoutedEdges)

  useEffect(() => {
    setNodes(layoutedNodes)
    setEdges(layoutedEdges)
  }, [layoutedNodes, layoutedEdges, setNodes, setEdges])

  const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    setSelectedStepId((prev) => (prev === node.id ? null : node.id))
  }, [])

  const selectedStep = useMemo(() => {
    if (!selectedStepId) return null
    return steps.find((s) => s.id === selectedStepId) || null
  }, [selectedStepId, steps])

  const copyConfigToClipboard = () => {
    if (!selectedStep?.config) return
    navigator.clipboard.writeText(formatJSON(selectedStep.config))
    setCopiedConfig(true)
    setTimeout(() => setCopiedConfig(false), 2000)
  }

  return (
    <div className="relative w-full h-[520px] bg-slate-50/70 rounded-lg border-[1.5px] border-slate-300/80 overflow-hidden select-none shadow-xs">
      {/* Top Left Canvas Info Pill */}
      <div className="absolute top-3 left-4 z-10 flex items-center space-x-2 pointer-events-none">
        <div className="bg-white/95 backdrop-blur-xs px-3 py-1.5 rounded-lg border border-slate-200/90 shadow-2xs flex items-center gap-2 text-xs pointer-events-auto">
          <Layers className="w-3.5 h-3.5 text-slate-500" />
          <span className="font-semibold text-slate-800">{workflowName}</span>
          <span className="text-slate-300">|</span>
          <span className="text-slate-500 font-mono text-[11px]">
            {steps.length} Nodes
          </span>
        </div>
      </div>

      {/* Interactive React Flow Canvas */}
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        onNodesChange={onNodesChange}
        onEdgesChange={onEdgesChange}
        onNodeClick={onNodeClick}
        fitView
        fitViewOptions={{ padding: 0.25 }}
        minZoom={0.4}
        maxZoom={1.8}
        className="bg-[#F8FAFC]"
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={18}
          size={1.2}
          color="#CBD5E1"
        />
        <Controls
          showInteractive={false}
          className="bg-white/95 backdrop-blur-xs border border-slate-200/90 rounded-lg shadow-2xs overflow-hidden"
        />
        <MiniMap
          nodeColor="#E2E8F0"
          maskColor="rgba(241, 245, 249, 0.7)"
          className="bg-white/90 border border-slate-200/90 rounded-lg shadow-2xs"
          zoomable
          pannable
        />
      </ReactFlow>

      {/* Slide-out Inspector Drawer for Selected Step */}
      {selectedStep && (
        <div className="absolute top-0 right-0 h-full w-84 bg-white/98 backdrop-blur-sm border-l border-slate-200 shadow-lg z-30 flex flex-col animate-in slide-in-from-right duration-200">
          {/* Drawer Header */}
          <div className="px-4 py-3 bg-slate-50/90 border-b border-slate-200 flex items-center justify-between">
            <div className="space-y-0.5">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-slate-500">
                Step Inspector
              </span>
              <h4 className="text-xs font-bold text-slate-900 truncate max-w-[200px]">
                {selectedStep.name}
              </h4>
            </div>
            <button
              onClick={() => setSelectedStepId(null)}
              className="p-1 text-slate-400 hover:text-slate-700 hover:bg-slate-200/60 rounded-md transition cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          {/* Drawer Content */}
          <div className="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
            {/* Basic Info */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-slate-500">Step ID:</span>
                <span className="font-mono font-medium text-slate-800">
                  {selectedStep.id}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-slate-500">Type:</span>
                <span className="px-2 py-0.5 rounded text-[11px] font-mono font-medium uppercase bg-slate-100 text-slate-700 border border-slate-200/80">
                  {selectedStep.type}
                </span>
              </div>
              {selectedStep.timeout && (
                <div className="flex items-center justify-between">
                  <span className="text-slate-500">Timeout:</span>
                  <span className="font-mono text-slate-800">
                    {selectedStep.timeout}s
                  </span>
                </div>
              )}
            </div>

            {/* Dependencies */}
            <div className="pt-2 border-t border-slate-100 space-y-1.5">
              <span className="text-slate-500 font-medium">Depends On:</span>
              {!selectedStep.depends_on || selectedStep.depends_on.length === 0 ? (
                <p className="text-slate-400 italic">None (Root Step)</p>
              ) : (
                <div className="flex flex-wrap gap-1">
                  {selectedStep.depends_on.map((depId) => (
                    <button
                      key={depId}
                      onClick={() => setSelectedStepId(depId)}
                      className="px-2 py-0.5 rounded bg-slate-100 hover:bg-indigo-50 hover:text-indigo-700 hover:border-indigo-300 border border-slate-200 font-mono text-[11px] text-slate-700 transition cursor-pointer"
                    >
                      {depId}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Retry Policy */}
            {selectedStep.retry && (
              <div className="pt-2 border-t border-slate-100 space-y-1.5">
                <span className="text-slate-500 font-medium">Retry Policy:</span>
                <div className="bg-slate-50 p-2.5 rounded-md border border-slate-200/80 space-y-1 font-mono text-[11px]">
                  <div className="flex justify-between">
                    <span className="text-slate-500">Max Attempts:</span>
                    <span className="font-bold text-slate-800">
                      {selectedStep.retry.max_attempts}x
                    </span>
                  </div>
                  {selectedStep.retry.initial_interval && (
                    <div className="flex justify-between">
                      <span className="text-slate-500">Initial Interval:</span>
                      <span className="text-slate-700">
                        {selectedStep.retry.initial_interval}s
                      </span>
                    </div>
                  )}
                  {selectedStep.retry.backoff_coefficient && (
                    <div className="flex justify-between">
                      <span className="text-slate-500">Backoff Coeff:</span>
                      <span className="text-slate-700">
                        {selectedStep.retry.backoff_coefficient}x
                      </span>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Rollback Details */}
            {selectedStep.compensation && (
              <div className="pt-2 border-t border-slate-100 space-y-1.5">
                <div className="flex items-center gap-1.5 font-medium text-amber-900">
                  <ShieldAlert className="w-3.5 h-3.5 text-amber-600" />
                  <span>Rollback: {selectedStep.compensation.name}</span>
                </div>
                <div className="bg-amber-50/50 p-2.5 rounded-md border border-amber-200/80 space-y-1 font-mono text-[11px]">
                  <div className="flex justify-between">
                    <span className="text-amber-800/80">Action ID:</span>
                    <span className="text-amber-950 font-medium">
                      {selectedStep.compensation.id}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-amber-800/80">Type:</span>
                    <span className="uppercase text-amber-950">
                      {selectedStep.compensation.type}
                    </span>
                  </div>
                  {selectedStep.compensation.config && (
                    <pre className="mt-1.5 p-2 bg-white border border-amber-200 rounded font-mono text-slate-800 text-[10px] overflow-x-auto">
                      {formatJSON(selectedStep.compensation.config)}
                    </pre>
                  )}
                </div>
              </div>
            )}

            {/* Step Config */}
            {selectedStep.config && (
              <div className="pt-2 border-t border-slate-100 space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className="text-slate-500 font-medium">Configuration:</span>
                  <button
                    onClick={copyConfigToClipboard}
                    className="p-1 hover:bg-slate-100 rounded text-slate-500 hover:text-slate-800 transition cursor-pointer flex items-center gap-1 text-[10px]"
                    title="Copy JSON"
                  >
                    {copiedConfig ? (
                      <>
                        <Check className="w-3 h-3 text-emerald-600" />
                        <span className="text-emerald-600">Copied</span>
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3" />
                        <span>Copy</span>
                      </>
                    )}
                  </button>
                </div>
                <pre className="p-2.5 bg-slate-50 border border-slate-200 rounded-md font-mono text-slate-800 text-[11px] overflow-x-auto max-h-48">
                  {formatJSON(selectedStep.config)}
                </pre>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
