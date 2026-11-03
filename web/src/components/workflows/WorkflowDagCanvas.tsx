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
  GitBranch,
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
      className={`w-[240px] rounded-lg border-[1.5px] p-3 shadow-xs transition-all select-none cursor-pointer flex flex-col justify-between ${
        isSelected
          ? 'border-neutral-400 ring-2 ring-neutral-500/30 shadow-md bg-[#181818]'
          : 'border-neutral-800 hover:border-neutral-700 bg-[#141414]'
      }`}
    >
      {/* Target Handle (Left) */}
      <Handle
        type="target"
        position={Position.Left}
        className="!w-2.5 !h-2.5 !bg-neutral-600 !border-2 !border-[#141414] hover:!bg-neutral-300"
      />

      {/* Top Header */}
      <div className="flex items-center justify-between gap-1 mb-1.5">
        <div className="flex items-center space-x-1">
          <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium uppercase bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
            {step.type}
          </span>
          {step.when && (
            <span
              className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium bg-indigo-950/50 text-indigo-400 border border-indigo-800/60"
              title="Conditional step (when)"
            >
              when
            </span>
          )}
        </div>

        <div className="flex items-center space-x-1">
          {step.retry?.max_attempts && (
            <span className="px-1.5 py-0.5 rounded text-[10px] font-mono font-medium text-neutral-400 bg-neutral-800/80 border border-neutral-700/60">
              {step.retry.max_attempts}x
            </span>
          )}
          {hasRollback && (
            <span
              className="px-1.5 py-0.5 rounded text-[10px] font-medium bg-amber-950/50 text-amber-400 border border-amber-800/60 flex items-center gap-0.5"
              title="Rollback handler defined"
            >
              <ShieldAlert className="w-2.5 h-2.5 text-amber-400" />
              <span>Rollback</span>
            </span>
          )}
        </div>
      </div>

      {/* Center Details */}
      <div className="space-y-0.5">
        <p className="font-bold text-xs text-neutral-100 truncate" title={step.name}>
          {step.name}
        </p>
        <p className="font-mono text-[10px] text-neutral-500 truncate" title={step.id}>
          {step.id}
        </p>
      </div>

      {/* Source Handle (Right) */}
      <Handle
        type="source"
        position={Position.Right}
        className="!w-2.5 !h-2.5 !bg-neutral-600 !border-2 !border-[#141414] hover:!bg-neutral-300"
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
              stroke: isEdgeActive ? '#EDEDED' : '#404040',
              strokeWidth: isEdgeActive ? 2.5 : 1.75,
            },
            markerEnd: {
              type: MarkerType.ArrowClosed,
              color: isEdgeActive ? '#EDEDED' : '#404040',
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
    <div className="relative w-full h-[520px] bg-[#0e0e0e] rounded-lg border-[1.5px] border-neutral-800 overflow-hidden select-none shadow-xs">
      {/* Top Left Canvas Info Pill */}
      <div className="absolute top-3 left-4 z-10 flex items-center space-x-2 pointer-events-none">
        <div className="bg-[#141414]/90 backdrop-blur-xs px-3 py-1.5 rounded-lg border border-neutral-800 shadow-2xs flex items-center gap-2 text-xs pointer-events-auto">
          <Layers className="w-3.5 h-3.5 text-neutral-400" />
          <span className="font-semibold text-neutral-200">{workflowName}</span>
          <span className="text-neutral-700">|</span>
          <span className="text-neutral-400 font-mono text-[11px]">
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
        className="bg-[#0a0a0a]"
      >
        <Background
          variant={BackgroundVariant.Dots}
          gap={18}
          size={1.2}
          color="#262626"
        />
        <Controls
          showInteractive={false}
          className="bg-[#141414] border border-neutral-800 rounded-lg shadow-2xs overflow-hidden [&_button]:!bg-[#141414] [&_button]:!border-neutral-800 [&_button]:!fill-neutral-300 hover:[&_button]:!bg-neutral-800"
        />
        <MiniMap
          bgColor="#141414"
          nodeColor="#262626"
          nodeStrokeColor="#404040"
          maskColor="rgba(10, 10, 10, 0.75)"
          className="!bg-[#141414] border border-neutral-800 rounded-lg shadow-2xs"
          zoomable
          pannable
        />
      </ReactFlow>

      {/* Slide-out Inspector Drawer for Selected Step */}
      {selectedStep && (
        <div className="absolute top-0 right-0 h-full w-84 bg-[#141414]/98 backdrop-blur-sm border-l border-neutral-800 shadow-2xl z-30 flex flex-col animate-in slide-in-from-right duration-200">
          {/* Drawer Header */}
          <div className="px-4 py-3 bg-[#181818] border-b border-neutral-800 flex items-center justify-between">
            <div className="space-y-0.5">
              <span className="text-[10px] font-semibold uppercase tracking-wider text-neutral-400">
                Step Inspector
              </span>
              <h4 className="text-xs font-bold text-neutral-100 truncate max-w-[200px]">
                {selectedStep.name}
              </h4>
            </div>
            <button
              onClick={() => setSelectedStepId(null)}
              className="p-1 text-neutral-400 hover:text-neutral-100 hover:bg-neutral-800 rounded-md transition cursor-pointer"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          {/* Drawer Content */}
          <div className="flex-1 overflow-y-auto p-4 space-y-4 text-xs">
            {/* Basic Info */}
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <span className="text-neutral-400">Step ID:</span>
                <span className="font-mono font-medium text-neutral-200">
                  {selectedStep.id}
                </span>
              </div>
              <div className="flex items-center justify-between">
                <span className="text-neutral-400">Type:</span>
                <span className="px-2 py-0.5 rounded text-[11px] font-mono font-medium uppercase bg-neutral-800/80 text-neutral-300 border border-neutral-700/60">
                  {selectedStep.type}
                </span>
              </div>
              {selectedStep.timeout && (
                <div className="flex items-center justify-between">
                  <span className="text-neutral-400">Timeout:</span>
                  <span className="font-mono text-neutral-200">
                    {selectedStep.timeout}s
                  </span>
                </div>
              )}
            </div>

            {/* Dependencies */}
            <div className="pt-2 border-t border-neutral-800 space-y-1.5">
              <span className="text-neutral-400 font-medium">Depends On:</span>
              {!selectedStep.depends_on || selectedStep.depends_on.length === 0 ? (
                <p className="text-neutral-500 italic">None (Root Step)</p>
              ) : (
                <div className="flex flex-wrap gap-1">
                  {selectedStep.depends_on.map((depId) => (
                    <button
                      key={depId}
                      onClick={() => setSelectedStepId(depId)}
                      className="px-2 py-0.5 rounded bg-neutral-800 hover:bg-neutral-700 hover:text-neutral-100 border border-neutral-700 font-mono text-[11px] text-neutral-300 transition cursor-pointer"
                    >
                      {depId}
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Retry Policy */}
            {selectedStep.retry && (
              <div className="pt-2 border-t border-neutral-800 space-y-1.5">
                <span className="text-neutral-400 font-medium">Retry Policy:</span>
                <div className="bg-[#0c0c0c] p-2.5 rounded-md border border-neutral-800 space-y-1 font-mono text-[11px]">
                  <div className="flex justify-between">
                    <span className="text-neutral-400">Max Attempts:</span>
                    <span className="font-bold text-neutral-200">
                      {selectedStep.retry.max_attempts}x
                    </span>
                  </div>
                  {selectedStep.retry.initial_interval && (
                    <div className="flex justify-between">
                      <span className="text-neutral-400">Initial Interval:</span>
                      <span className="text-neutral-300">
                        {selectedStep.retry.initial_interval}s
                      </span>
                    </div>
                  )}
                  {selectedStep.retry.backoff_coefficient && (
                    <div className="flex justify-between">
                      <span className="text-neutral-400">Backoff Coeff:</span>
                      <span className="text-neutral-300">
                        {selectedStep.retry.backoff_coefficient}x
                      </span>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Rollback Details */}
            {selectedStep.compensation && (
              <div className="pt-2 border-t border-neutral-800 space-y-1.5">
                <div className="flex items-center gap-1.5 font-medium text-amber-400">
                  <ShieldAlert className="w-3.5 h-3.5 text-amber-400" />
                  <span>Rollback: {selectedStep.compensation.name}</span>
                </div>
                <div className="bg-neutral-900 p-2.5 rounded-md border border-neutral-700/80 space-y-1 font-mono text-[11px]">
                  <div className="flex justify-between">
                    <span className="text-neutral-400">Action ID:</span>
                    <span className="text-neutral-200 font-medium">
                      {selectedStep.compensation.id}
                    </span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-neutral-400">Type:</span>
                    <span className="uppercase text-neutral-200">
                      {selectedStep.compensation.type}
                    </span>
                  </div>
                  {selectedStep.compensation.config && (
                    <pre className="mt-1.5 p-2 bg-[#0c0c0c] border border-neutral-800 rounded font-mono text-neutral-300 text-[10px] overflow-x-auto">
                      {formatJSON(selectedStep.compensation.config)}
                    </pre>
                  )}
                </div>
              </div>
            )}

            {/* When Condition */}
            {selectedStep.when && (
              <div className="pt-2 border-t border-neutral-800 space-y-1.5">
                <div className="flex items-center gap-1.5 font-medium text-indigo-400">
                  <GitBranch className="w-3.5 h-3.5 text-indigo-400" />
                  <span>Condition (When):</span>
                </div>
                <pre className="p-2.5 bg-[#0c0c0c] border border-indigo-950/60 text-indigo-300 rounded font-mono text-[10px] overflow-x-auto">
                  {formatJSON(selectedStep.when)}
                </pre>
              </div>
            )}

            {/* Step Config */}
            {selectedStep.config && (
              <div className="pt-2 border-t border-neutral-800 space-y-1.5">
                <div className="flex items-center justify-between">
                  <span className="text-neutral-400 font-medium">Configuration:</span>
                  <button
                    onClick={copyConfigToClipboard}
                    className="p-1 hover:bg-neutral-800 rounded text-neutral-400 hover:text-neutral-200 transition cursor-pointer flex items-center gap-1 text-[10px]"
                    title="Copy JSON"
                  >
                    {copiedConfig ? (
                      <>
                        <Check className="w-3 h-3 text-emerald-400" />
                        <span className="text-emerald-400">Copied</span>
                      </>
                    ) : (
                      <>
                        <Copy className="w-3 h-3" />
                        <span>Copy</span>
                      </>
                    )}
                  </button>
                </div>
                <pre className="p-2.5 bg-[#0c0c0c] border border-neutral-800 rounded-md font-mono text-neutral-300 text-[11px] overflow-x-auto max-h-48">
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
