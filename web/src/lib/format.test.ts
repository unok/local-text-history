import { describe, it, expect } from 'vitest'
import { formatBytes, formatDate } from './format'

describe('formatBytes', () => {
  it('returns "0 B" for zero', () => {
    expect(formatBytes(0)).toBe('0 B')
  })

  it('returns "0 B" for negative values', () => {
    expect(formatBytes(-1)).toBe('0 B')
    expect(formatBytes(-1024)).toBe('0 B')
  })

  it('formats bytes', () => {
    expect(formatBytes(1)).toBe('1.0 B')
    expect(formatBytes(512)).toBe('512.0 B')
  })

  it('formats kilobytes', () => {
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes(1536)).toBe('1.5 KB')
  })

  it('formats megabytes', () => {
    expect(formatBytes(1048576)).toBe('1.0 MB')
    expect(formatBytes(5242880)).toBe('5.0 MB')
  })

  it('formats gigabytes', () => {
    expect(formatBytes(1073741824)).toBe('1.0 GB')
  })

  it('formats terabytes', () => {
    expect(formatBytes(1099511627776)).toBe('1.0 TB')
  })

  it('clamps to TB for values larger than TB', () => {
    // 1 PB = 1024 TB — should still display in TB
    const petabyte = 1125899906842624
    const result = formatBytes(petabyte)
    expect(result).toContain('TB')
    expect(result).toBe('1024.0 TB')
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
