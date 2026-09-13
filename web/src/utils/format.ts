export function formatOtaProgress(j: { status?: string; step?: number; description?: string }) {
  const status = j.status || '';
  const step = Number(j.step) || 0;
  const desc = (j.description || '').trim();

  let head = '';
  if (status === 'done' || step >= 101) head = 'finish';
  else if (status === 'cancelled' || step === -4) head = 'cancelled';
  else if (status === 'failed' || step < 0) head = 'failed';
  else if (step > 0 && step <= 100) head = `${step}%`;
  else if (status === 'pending') head = 'queued';

  if (head && desc && desc.toLowerCase() !== head) return `${head} · ${desc}`;
  return head || desc || '—';
}

export function formatBytes(n: unknown) {
  const v = Number(n);
  if (!Number.isFinite(v)) return String(n);
  const abs = Math.abs(v);
  if (abs >= 1024 ** 3) return (v / 1024 ** 3).toFixed(2) + ' GiB';
  if (abs >= 1024 ** 2) return (v / 1024 ** 2).toFixed(2) + ' MiB';
  if (abs >= 1024) return (v / 1024).toFixed(1) + ' KiB';
  return Math.round(v) + ' B';
}

export function formatDuration(sec: unknown) {
  let s = Math.max(0, Math.floor(Number(sec) || 0));
  const d = Math.floor(s / 86400);
  s %= 86400;
  const h = Math.floor(s / 3600);
  s %= 3600;
  const m = Math.floor(s / 60);
  s %= 60;
  const parts: string[] = [];
  if (d) parts.push(d + 'd');
  if (h || d) parts.push(h + 'h');
  if (m || h || d) parts.push(m + 'm');
  parts.push(s + 's');
  return parts.join(' ');
}

export function formatAge(ts?: number | null) {
  if (!ts) return '';
  const sec = Math.max(0, Math.floor((Date.now() - ts) / 1000));
  if (sec < 5) return 'just now';
  if (sec < 60) return sec + 's ago';
  if (sec < 3600) return Math.floor(sec / 60) + 'm ago';
  return new Date(ts).toLocaleString();
}

export function sampleValue(s: {
  value_num?: number | null;
  value_text?: string | null;
  value_bool?: boolean | null;
} | null) {
  if (!s) return null;
  if (s.value_num != null) return s.value_num;
  if (s.value_bool != null) return s.value_bool;
  if (s.value_text != null) return s.value_text;
  return null;
}

export function humanizeKey(key: string) {
  return key
    .replace(/([a-z])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

export function propLabel(label: string | undefined, key: string) {
  const l = (label || '').trim();
  const k = (key || '').trim();
  if (!l || l === k) return { primary: k, secondary: '' };
  return { primary: l, secondary: k };
}
