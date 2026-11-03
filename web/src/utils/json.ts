function toComparable(str: string): string | null {
  const match = str.match(/^([+-]?)(\d+)?(?:\.(\d+))?(?:[eE]([+-]?\d+))?$/)
  if (!match) return null
  const sign = match[1] === '-' ? '-' : ''
  const intPart = match[2] || '0'
  const fracPart = match[3] || ''
  const exp = parseInt(match[4] || '0', 10)

  const combined = intPart + fracPart
  const decPos = intPart.length + exp

  let intOut = ''
  let fracOut = ''

  if (decPos <= 0) {
    intOut = '0'
    fracOut = '0'.repeat(-decPos) + combined
  } else if (decPos >= combined.length) {
    intOut = combined + '0'.repeat(decPos - combined.length)
    fracOut = ''
  } else {
    intOut = combined.slice(0, decPos)
    fracOut = combined.slice(decPos)
  }

  intOut = intOut.replace(/^0+/, '') || '0'
  fracOut = fracOut.replace(/0+$/, '')

  return sign + intOut + (fracOut ? '.' + fracOut : '')
}

/**
 * losesPrecision returns true if converting the number token string into a standard
 * JavaScript IEEE 754 float64 (Number) loses precision.
 * This covers large integers (> 2^53 - 1), decimals (.0), scientific notation (e15),
 * and high-precision fractional numbers.
 */
export function losesPrecision(numStr: string): boolean {
  const n = Number(numStr)
  if (!Number.isFinite(n)) return true
  const original = toComparable(numStr)
  const parsedBack = toComparable(String(n))
  return original !== parsedBack
}

/**
 * parseLosslessJSON parses a JSON string while preserving the exact precision
 * of numbers that lose precision when parsed by JavaScript's standard IEEE 754 float64 Number.
 * Such numbers (including large integers, decimals with trailing zeros or large mantissas,
 * and scientific exponent notation) are safely preserved as string representations.
 */
export function parseLosslessJSON<T = any>(jsonText: string): T {
  let result = ''
  let inString = false
  let isEscaped = false
  let i = 0

  while (i < jsonText.length) {
    const ch = jsonText[i]

    if (inString) {
      result += ch
      if (isEscaped) {
        isEscaped = false
      } else if (ch === '\\') {
        isEscaped = true
      } else if (ch === '"') {
        inString = false
      }
      i++
      continue
    }

    if (ch === '"') {
      inString = true
      result += ch
      i++
      continue
    }

    // Outside string literal: detect number token
    if ((ch === '-' && i + 1 < jsonText.length && /\d/.test(jsonText[i + 1])) || /\d/.test(ch)) {
      let numStr = ''
      while (i < jsonText.length && /[-+eE.\d]/.test(jsonText[i])) {
        numStr += jsonText[i]
        i++
      }

      if (losesPrecision(numStr)) {
        result += `"${numStr}"`
      } else {
        result += numStr
      }
      continue
    }

    result += ch
    i++
  }

  return JSON.parse(result) as T
}
