// Форматирование чисел по правилам дизайн-системы:
// пробел между разрядами, запятая в дробях — «1 240 м²», «26,15 м²».

const NBSP = ' ';

export function fmtNum(n: number): string {
  const r = Math.round(n * 100) / 100;
  const [int, frac] = String(Math.abs(r)).split('.');
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, NBSP);
  return (r < 0 ? '−' : '') + grouped + (frac ? ',' + frac : '');
}

export function fmtM2(n: number): string {
  return fmtNum(n) + NBSP + 'м²';
}

/** Площади из API приходят десятичными строками ("26.15"): только для показа, не для расчётов. */
export function fmtM2Str(s: string): string {
  return fmtM2(Number(s));
}

/** Оценка «~N кв.» по средней площади квартиры. */
export function approxFlats(m2: number, avgArea: number): number {
  return Math.max(1, Math.ceil(m2 / avgArea));
}

export function pct(part: number, total: number): number {
  return Math.round((part / total) * 100);
}
