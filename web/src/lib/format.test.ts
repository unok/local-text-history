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
  it('formats unix timestamp as YYYY/MM/DD HH:MM:SS', () => {
    // 2024-01-01T00:00:00Z = 1704067200
    const unix = 1704067200
    const d = new Date(unix * 1000)
    const expected =
      `${d.getFullYear()}/${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')} ` +
      `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
    expect(formatDate(unix)).toBe(expected)
  })

  it('zero-pads single-digit month, day, hours, minutes, seconds', () => {
    // 2024-03-05T07:08:09Z = pick a timestamp where local time has single digits
    // Use a known date and compute expected from local timezone
    const unix = 1709618889 // 2024-03-05T07:08:09Z
    const d = new Date(unix * 1000)
    const result = formatDate(unix)
    expect(result).toMatch(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}$/)
    // Verify each component matches
    expect(result).toBe(
      `${d.getFullYear()}/${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')} ` +
      `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
    )
  })

  it('handles zero timestamp (epoch)', () => {
    const d = new Date(0)
    const expected =
      `${d.getFullYear()}/${String(d.getMonth() + 1).padStart(2, '0')}/${String(d.getDate()).padStart(2, '0')} ` +
      `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}:${String(d.getSeconds()).padStart(2, '0')}`
    expect(formatDate(0)).toBe(expected)
  })

  it('always matches YYYY/MM/DD HH:MM:SS format', () => {
    const timestamps = [0, 1704067200, 1709618889, 1700000000]
    for (const ts of timestamps) {
      expect(formatDate(ts)).toMatch(/^\d{4}\/\d{2}\/\d{2} \d{2}:\d{2}:\d{2}$/)
    }
  })
})
