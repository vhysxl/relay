export type StepType = 'http' | 'message' | string

export type ExecutionStatus =
  | 'PENDING'
  | 'RUNNING'
  | 'COMPLETED'
  | 'FAILED'
  | 'COMPENSATING'
  | 'COMPENSATED'
  | 'COMPENSATION_FAILED'
  | string

export interface RetryPolicy {
  max_attempts?: number
  initial_interval?: number
  max_interval?: number
  backoff_coefficient?: number
}

export interface Step {
  id: string
  name: string
  type: StepType
  depends_on?: string[]
  config?: any
  compensation?: Step
  retry?: RetryPolicy
  timeout?: number
}

export interface Workflow {
  id: string
  name: string
  version: number
  created_at: string
  steps?: Step[]
}

export interface StepExecution {
  id?: string
  step_id: string
  name?: string
  type: StepType
  status: ExecutionStatus
  attempt?: number
  max_attempts?: number
  timeout_seconds?: number
  depends_on?: string[]
  config?: any
  output?: any
  error_message?: string
  is_compensation?: boolean
  executed_at?: string
}

export interface Execution {
  id: string
  workflow_id: string
  workflow_name?: string
  name?: string
  type?: string
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
