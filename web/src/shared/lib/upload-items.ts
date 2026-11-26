export interface UploadItem {
  file: File
  /** Path below the confirmed target directory, "/"-separated. */
  relativePath: string
}

export const FILE_LIMIT = 1024 ** 3
export const TREE_ENTRY_LIMIT = 10000
export const TREE_DEPTH_LIMIT = 32

/** Mirrors the server's rule: every segment is a plain, non-empty name. */
export function validRelativePath(relativePath: string): boolean {
  if (!relativePath || relativePath.length > 4096 || relativePath.startsWith('/')) return false
  const segments = relativePath.split('/')
  return (
    segments.length <= TREE_DEPTH_LIMIT &&
    segments.every(
      (segment) =>
        segment !== '' &&
        segment !== '.' &&
        segment !== '..' &&
        !/[\\\0]/.test(segment) &&
        new TextEncoder().encode(segment).length <= 255,
    )
  )
}

export function fromFileList(files: File[]): UploadItem[] {
  return files.map((file) => ({
    file,
    relativePath: file.webkitRelativePath || file.name,
  }))
}

export function topLevelFolders(items: UploadItem[]): string[] {
  return [
    ...new Set(
      items
        .filter((item) => item.relativePath.includes('/'))
        .map((item) => item.relativePath.split('/')[0]),
    ),
  ]
}

export function checkItems(items: UploadItem[]): string | null {
  if (items.length === 0) return '没有可上传的文件。'
  if (items.length > TREE_ENTRY_LIMIT) return '一次最多上传 10,000 个文件。'
  if (items.some((item) => item.file.size > FILE_LIMIT)) return '单个文件不得超过 1 GiB。'
  const invalid = items.find((item) => !validRelativePath(item.relativePath))
  if (invalid) return `文件名或目录层级不受支持：${invalid.relativePath}`
  return null
}

function readEntries(reader: FileSystemDirectoryReader): Promise<FileSystemEntry[]> {
  return new Promise((resolve, reject) => reader.readEntries(resolve, reject))
}

function entryFile(entry: FileSystemFileEntry): Promise<File> {
  return new Promise((resolve, reject) => entry.file(resolve, reject))
}

/**
 * Expands dropped files and folders. Symlinked or unreadable entries reject
 * the whole drop instead of silently uploading a partial tree.
 */
export async function fromDataTransfer(transfer: DataTransfer): Promise<UploadItem[]> {
  const entries = Array.from(transfer.items)
    .map((item) => item.webkitGetAsEntry?.())
    .filter((entry): entry is FileSystemEntry => !!entry)
  if (entries.length === 0) return fromFileList(Array.from(transfer.files))
  const items: UploadItem[] = []
  async function visit(entry: FileSystemEntry, prefix: string, depth: number): Promise<void> {
    if (depth > TREE_DEPTH_LIMIT) throw new Error('目录层级超过 32 层。')
    if (items.length > TREE_ENTRY_LIMIT) throw new Error('一次最多上传 10,000 个文件。')
    const relativePath = prefix ? `${prefix}/${entry.name}` : entry.name
    if (entry.isFile) {
      items.push({ file: await entryFile(entry as FileSystemFileEntry), relativePath })
      return
    }
    if (!entry.isDirectory) return
    const reader = (entry as FileSystemDirectoryEntry).createReader()
    for (;;) {
      const batch = await readEntries(reader)
      if (batch.length === 0) break
      for (const child of batch) await visit(child, relativePath, depth + 1)
    }
  }
  for (const entry of entries) await visit(entry, '', 1)
  return items
}
