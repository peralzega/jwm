/** Kunci cache daftar aturan otomasi board (dipisah agar panel tetap dimuat terpisah). */
export const automationsKey = (boardId: string) => ['automations', boardId] as const
