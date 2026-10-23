import type { Step } from '../types/relay'

/**
 * Sorts steps topologically based on their `depends_on` relationships.
 * Root steps (no dependencies) appear first, and dependent steps follow.
 * If steps have no dependency differences, original order is preserved.
 */
export function sortStepsTopologically(steps: Step[]): Step[] {
  if (!steps || steps.length <= 1) return steps || []

  const stepMap = new Map<string, Step>()
  const inDegree = new Map<string, number>()
  const dependents = new Map<string, string[]>()

  steps.forEach((step) => {
    stepMap.set(step.id, step)
    inDegree.set(step.id, step.depends_on?.length || 0)
    dependents.set(step.id, [])
  })

  steps.forEach((step) => {
    if (step.depends_on) {
      step.depends_on.forEach((dep) => {
        if (dependents.has(dep)) {
          dependents.get(dep)!.push(step.id)
        }
      })
    }
  })

  // Start with steps that have in-degree 0 (no prerequisites)
  const queue: string[] = []
  steps.forEach((step) => {
    if ((inDegree.get(step.id) || 0) === 0) {
      queue.push(step.id)
    }
  })

  const sorted: Step[] = []
  while (queue.length > 0) {
    const curr = queue.shift()!
    const s = stepMap.get(curr)
    if (s) sorted.push(s)

    const nextSteps = dependents.get(curr) || []
    for (const next of nextSteps) {
      const deg = (inDegree.get(next) || 0) - 1
      inDegree.set(next, deg)
      if (deg === 0) {
        queue.push(next)
      }
    }
  }

  // Fallback for any unvisited nodes (e.g. cycle or disconnected)
  if (sorted.length < steps.length) {
    const visited = new Set(sorted.map((s) => s.id))
    steps.forEach((s) => {
      if (!visited.has(s.id)) {
        sorted.push(s)
      }
    })
  }

  return sorted
}
