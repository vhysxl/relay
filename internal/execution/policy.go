package execution

import "math"

// MySQL DATE_ADD and TIMESTAMP have finite ranges. Cap unbounded policies to
// one year instead of overflowing float/int conversions or generating NULL dates.
const maximumBackoffSeconds = 365 * 24 * 60 * 60

func retryDelay(s StepExecution) int {
	limit := maximumBackoffSeconds
	if s.MaxBackoffSeconds > 0 && s.MaxBackoffSeconds < limit {
		limit = s.MaxBackoffSeconds
	}
	if s.BackoffSeconds <= 0 {
		return 0
	}
	multiplier := s.Multiplier
	if multiplier <= 0 || math.IsNaN(multiplier) {
		multiplier = 1
	}
	power := max(s.Attempt-1, 0)
	delay := float64(s.BackoffSeconds) * math.Pow(multiplier, float64(power))
	if math.IsInf(delay, 1) || math.IsNaN(delay) || delay >= float64(limit) {
		return limit
	}
	return int(math.Round(max(delay, 0)))
}

func mayRetry(workflowStatus WorkflowStatus, s StepExecution) bool {
	return s.Attempt < s.MaxAttempts &&
		((!s.IsCompensation && workflowStatus == StatusRunning) ||
			(s.IsCompensation && workflowStatus == StatusCompensating))
}

func active(status StepStatus) bool {
	return status == StatusWaiting || status == StatusPending || status == StatusRunning
}

// reconcile decides transitions using a consistent workflow snapshot. It has no
// SQL or side effects; the caller persists its decisions under the workflow lock.
func reconcile(status WorkflowStatus, steps []StepExecution) (WorkflowStatus, []StepExecution) {
	result := append([]StepExecution(nil), steps...)
	normal := make(map[string]StepExecution)
	comp := make(map[string]StepExecution)
	children := make(map[string][]string)
	failed := status == StatusCompensating || status == StatusFailed || status == StatusCompensationFailed
	compFailed := status == StatusCompensationFailed
	for _, s := range steps {
		if s.IsCompensation {
			comp[s.StepID] = s
			compFailed = compFailed || s.Status == StatusCompensationFailed
		} else {
			normal[s.StepID] = s
			failed = failed || s.Status == StatusFailed
			for _, dep := range s.DependsOn {
				children[dep] = append(children[dep], s.StepID)
			}
		}
	}
	if status == StatusCompleted || status == StatusCompensated {
		return status, result
	}
	if compFailed {
		failed = true
	}
	if !failed {
		allComplete := true
		for i, s := range result {
			if s.IsCompensation {
				continue
			}
			allComplete = allComplete && s.Status == StatusCompleted
			if s.Status == StatusWaiting && dependenciesComplete(s.DependsOn, normal) {
				result[i].Status = StatusPending
			}
		}
		if allComplete {
			for i, s := range result {
				if s.IsCompensation && active(s.Status) && s.Status != StatusRunning {
					result[i].Status = StatusCancelled
				}
			}
			return StatusCompleted, result
		}
		return status, result
	}
	// Apply normal cancellations before examining their compensation tickets.
	for i, s := range result {
		if !s.IsCompensation && (s.Status == StatusPending || s.Status == StatusWaiting) {
			result[i].Status = StatusCancelled
			normal[s.StepID] = result[i]
		}
	}
	for i, s := range result {
		if s.Status != StatusPending && s.Status != StatusWaiting {
			continue
		}
		if !s.IsCompensation || compFailed {
			result[i].Status = StatusCancelled
			continue
		}
		original, exists := normal[s.StepID]
		if !exists || original.Status == StatusFailed || original.Status == StatusCancelled {
			result[i].Status = StatusCancelled
		} else {
			// Re-evaluate legacy PENDING tickets too: a running original is unsafe.
			result[i].Status = StatusWaiting
		}
	}
	if compFailed {
		return StatusCompensationFailed, result
	}
	for _, s := range result {
		if s.IsCompensation {
			comp[s.StepID] = s
		}
	}
	running, remaining, compensated := false, false, false
	for i, s := range result {
		if !s.IsCompensation {
			running = running || s.Status == StatusRunning
			continue
		}
		compensated = compensated || s.Status == StatusCompensated
		remaining = remaining || active(s.Status)
		if s.Status == StatusWaiting && normal[s.StepID].Status == StatusCompleted &&
			rollbackDependenciesDone(s, children, normal, comp) {
			result[i].Status = StatusPending
		}
	}
	if running || remaining {
		return StatusCompensating, result
	}
	if compensated {
		return StatusCompensated, result
	}
	return StatusFailed, result
}

func dependenciesComplete(deps []string, normal map[string]StepExecution) bool {
	for _, id := range deps {
		if normal[id].Status != StatusCompleted {
			return false
		}
	}
	return true
}

func rollbackDependenciesDone(s StepExecution, children map[string][]string, normal, comp map[string]StepExecution) bool {
	// Traverse through steps without compensation as well, so A -> B -> C
	// rolls C back before A even when B has no compensation of its own.
	queue := append(append([]string{}, children[s.StepID]...), s.DependsOn...)
	seen := map[string]bool{s.StepID: true}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if seen[id] {
			continue
		}
		seen[id] = true
		if normal[id].Status == StatusRunning || active(comp[id].Status) {
			return false
		}
		queue = append(queue, children[id]...)
	}
	return true
}
