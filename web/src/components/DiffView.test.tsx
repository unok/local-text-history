import { describe, expect, it } from 'vitest'
import { buildDiff2HtmlConfig } from './DiffView'
import { ColorSchemeType } from 'diff2html/lib/types'

describe('buildDiff2HtmlConfig', () => {
  describe('colorScheme', () => {
    it('should use DARK colorScheme when theme is dark', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'dark')

      expect(config.colorScheme).toBe(ColorSchemeType.DARK)
    })

    it('should use LIGHT colorScheme when theme is light', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.colorScheme).toBe(ColorSchemeType.LIGHT)
    })

    it('should use LIGHT colorScheme for unknown theme values', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'system')

      expect(config.colorScheme).toBe(ColorSchemeType.LIGHT)
    })
  })

  describe('outputFormat', () => {
    it('should pass side-by-side format', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.outputFormat).toBe('side-by-side')
    })

    it('should pass line-by-line format', () => {
      const config = buildDiff2HtmlConfig('line-by-line', 'light')

      expect(config.outputFormat).toBe('line-by-line')
    })
  })

  describe('syntax highlighting options', () => {
    it('should enable highlight', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.highlight).toBe(true)
    })

    it('should enable synchronised scroll', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.synchronisedScroll).toBe(true)
    })
  })

  describe('disabled features', () => {
    it('should disable file list', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.drawFileList).toBe(false)
      expect(config.fileListToggle).toBe(false)
    })

    it('should disable file content toggle', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.fileContentToggle).toBe(false)
    })

    it('should disable sticky file headers', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.stickyFileHeaders).toBe(false)
    })
  })

  describe('matching strategy', () => {
    it('should use lines matching', () => {
      const config = buildDiff2HtmlConfig('side-by-side', 'light')

      expect(config.matching).toBe('lines')
    })
  })
})
