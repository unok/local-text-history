import { describe, it, expect } from 'vitest'
import { formatBytes, formatDate } from './format'

describe('formatBytes', () => {
  it('returns "0" for zero', () => {
    expect(formatBytes(0)).toBe('0')
  })

  it('returns "0" for negative values', () => {
    expect(formatBytes(-1)).toBe('0')
    expect(formatBytes(-1024)).toBe('0')
  })

  it('formats small values without commas', () => {
    expect(formatBytes(1)).toBe('1')
    expect(formatBytes(512)).toBe('512')
  })

  it('formats with comma separators', () => {
    expect(formatBytes(1024)).toBe('1,024')
    expect(formatBytes(1536)).toBe('1,536')
  })

  it('formats larger values with comma separators', () => {
    expect(formatBytes(1048576)).toBe('1,048,576')
    expect(formatBytes(5242880)).toBe('5,242,880')
  })

  it('formats gigabyte-scale values', () => {
    expect(formatBytes(1073741824)).toBe('1,073,741,824')
  })

  it('formats terabyte-scale values', () => {
    expect(formatBytes(1099511627776)).toBe('1,099,511,627,776')
  })
})

describe('formatDate', () => {
  it('converts unix timestamp to locale string', () => {
    // 2024-01-01T00:00:00Z = 1704067200
    const result = formatDate(1704067200)
    // The exact format depends on locale, but it should be a non-empty string
    expect(result).toBeTruthy()
    expect(typeof result).toBe('string')
  })

  it('handles zero timestamp (epoch)', () => {
    const result = formatDate(0)
    expect(result).toBeTruthy()
    // Should represent Jan 1, 1970 in some locale format
    expect(typeof result).toBe('string')
  })
})
