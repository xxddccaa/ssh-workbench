import { describe, expect, it } from 'vitest'
import { checkItems, fromFileList, topLevelFolders, validRelativePath } from './upload-items'

describe('folder upload paths', () => {
  it('accepts nested plain names including Chinese and spaces', () => {
    expect(validRelativePath('项目/子 目录/文件.txt')).toBe(true)
    expect(validRelativePath('a.txt')).toBe(true)
  })

  it('rejects traversal, absolute, empty and backslash segments', () => {
    for (const path of ['../a', 'a/../b', '/etc/passwd', 'a//b', 'a/./b', 'a\\b', '', 'a/\0'])
      expect(validRelativePath(path)).toBe(false)
    expect(validRelativePath(Array.from({ length: 33 }, () => 'd').join('/'))).toBe(false)
  })

  it('uses webkitRelativePath from directory pickers and lists root folders', () => {
    const file = new File(['x'], 'b.txt')
    Object.defineProperty(file, 'webkitRelativePath', { value: 'root/sub/b.txt' })
    const items = fromFileList([file, new File(['y'], 'plain.txt')])
    expect(items.map((item) => item.relativePath)).toEqual(['root/sub/b.txt', 'plain.txt'])
    expect(topLevelFolders(items)).toEqual(['root'])
    expect(checkItems(items)).toBeNull()
    expect(checkItems([])).not.toBeNull()
  })
})
