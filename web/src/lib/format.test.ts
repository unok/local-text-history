import { describe, it, expect } from 'vitest'
import { formatBytes, formatDateTime } from './format'

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

// Tests run with TZ=UTC (configured in vite.config.ts)
describe('formatDateTime', () => {
  it('formats unix timestamp as YYYY/MM/DD HH:MM:SS', () => {
    // 2024-01-01T00:00:00Z = 1704067200
    expect(formatDateTime(1704067200)).toBe('2024/01/01 00:00:00')
  })

  it('zero-pads single-digit month, day, hours, minutes, seconds', () => {
    // 2024-03-05T06:08:09Z = 1709618889
    expect(formatDateTime(1709618889)).toBe('2024/03/05 06:08:09')
  })

  it('handles zero timestamp (epoch)', () => {
    expect(formatDateTime(0)).toBe('1970/01/01 00:00:00')
  })

  it('always matches YYYY/MM/DD HH:MM:SS format', () => {
    const timestamps = [0, 1704067200, 1709618889, 1700000000]
    for (const ts of timestamps) {
      expect(formatDateTime(ts)).toMatch(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}$/)
    }
  })
})
