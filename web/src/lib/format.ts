export function formatBytes(bytes: number): string {
  if (bytes <= 0) return '0'
  return bytes.toLocaleString('en-US')
}

export function formatDate(unix: number): string {
  return new Date(unix * 1000).toLocaleString()
}
