/**
 * parseLosslessJSON parses a JSON string while preserving the exact precision
 * of large integers that exceed JavaScript's Number.MAX_SAFE_INTEGER (9007199254740991)
 * or are smaller than Number.MIN_SAFE_INTEGER (-9007199254740991).
 * Such numbers are safely converted into string representations so they are not rounded by IEEE 754 float64.
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

      // Check if it's an integer that exceeds safe integer range
      if (/^-?\d+$/.test(numStr)) {
        try {
          const big = BigInt(numStr)
          if (big > BigInt(Number.MAX_SAFE_INTEGER) || big < BigInt(Number.MIN_SAFE_INTEGER)) {
            result += `"${numStr}"`
          } else {
            result += numStr
          }
        } catch {
          result += `"${numStr}"`
        }
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
