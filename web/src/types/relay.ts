export type StepType = 'http' | 'message' | 'activity'

export type ExecutionStatus =
  | 'PENDING'
  | 'RUNNING'
  | 'COMPLETED'
  | 'SKIPPED'
  | 'FAILED'
  | 'COMPENSATING'
  | 'COMPENSATED'
  | 'COMPENSATION_FAILED'
  | string

export interface RetryPolicy {
  max_attempts?: number
  backoff?: number
  max_backoff?: number
  multiplier?: number
}

export interface ConditionCheckDetail {
  ref: string
  op: string
  expected?: any
  actual?: any
  missing?: boolean
  result?: boolean
  error?: string
}

export interface ConditionEvaluationAudit {
  evaluated_at: string
  result?: boolean
  error?: string
  checks?: ConditionCheckDetail[]
}

export interface Step {
  id: string
  name: string
  type: StepType
  depends_on?: string[]
  config?: any
  when?: any
  compensation?: Step
  retry?: RetryPolicy
  timeout?: number
}

interface WorkflowMetadata {
  id: string
  name: string
  version: number
  created_at: string
  updated_at?: string
}

export interface WorkflowSummary extends WorkflowMetadata {
  step_count: number
}

export interface Workflow extends WorkflowMetadata {
  context?: any
  steps?: Step[]
}

export type ChangeKind = 'added' | 'removed' | 'modified'

export interface WorkflowChange {
  path: string
  kind: ChangeKind
  before?: any
  after?: any
}

export interface WorkflowVersionChangesResponse {
  from_version: number | null
  to_version: number
  changes: WorkflowChange[]
}

export interface StepExecutionAttempt {
  attempt: number
  service_id: string
  worker_id: string
  started_at: string
  deadline: string
  finished_at?: string | null
  outcome: string
  error_message?: string | null
  report_type?: string | null
}

export interface StepExecution {
  id?: string
  step_id: string
  name?: string
  type: StepType
  status: ExecutionStatus
  task_queue?: string
  activity_name?: string
  attempt?: number
  max_attempts?: number
  attempts?: StepExecutionAttempt[]
  timeout_seconds?: number
  depends_on?: string[]
  config?: any
  when?: any
  condition_evaluation?: ConditionEvaluationAudit
  output?: any
  error_message?: string
  is_compensation?: boolean
  executed_at?: string
  created_at?: string
  updated_at?: string
}

export interface Execution {
  id: string
  workflow_id: string
  workflow_name?: string
  status: ExecutionStatus
  context?: any
  error_message?: string
  created_at: string
  updated_at?: string
  steps?: StepExecution[]
}

export type ViewMode = 'dashboard' | 'workflows' | 'workflow-detail' | 'history' | 'execution-detail'

export interface SystemMetrics {
  cpu_percent: number
  memory_used_mb: number
  memory_total_mb: number
  memory_percent: number
  go_alloc_mb: number
  go_sys_mb: number
  goroutines: number
  num_cpu: number
  uptime_seconds: number
}
