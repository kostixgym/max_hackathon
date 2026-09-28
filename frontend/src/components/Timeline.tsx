import { Typography as MaxTypography } from '@maxhub/max-ui';
// Таймлайн стадий инициативы: Черновик → Опрос → Требование → Собрание → Завершена.
// Перенос компонента Timeline из дизайна.

export const STAGES = ['Черновик', 'Опрос', 'Требование', 'Собрание', 'Завершена'] as const;

type Props = {
  /** индекс текущей стадии, 0–4 */
  stage: number;
  cancelled?: boolean;
  compact?: boolean;
  /** подписи под стадиями в полном виде */
  caps?: string[];
};

export function Timeline({ stage, cancelled = false, compact = false, caps = [] }: Props) {
  const rows = STAGES.map((name, i) => {
    const k = i < stage ? 'd' : i === stage ? (cancelled ? 'x' : stage === 4 ? 'd' : 'c') : 'f';
    return {
      name: i === stage && cancelled ? `${name} — отменена` : name,
      k,
      sym: k === 'd' ? '✓' : k === 'x' ? '×' : String(i + 1),
      cap: caps[i] ?? '',
    };
  });

  if (compact) {
    const now = cancelled ? `Отменена на этапе «${STAGES[stage]}»` : stage === 4 ? 'Завершена' : `Шаг ${stage + 1} из 5 · ${STAGES[stage]}`;
    const next = cancelled || stage >= 4 ? '' : `Далее: ${STAGES[stage + 1]}`;
    return (
      <div className="col" style={{ gap: 10 }}>
        <div className="between" style={{ alignItems: 'baseline' }}>
          <span style={{ fontSize: 16, lineHeight: '22px', fontWeight: 600 }}>{now}</span>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">{next}</MaxTypography.Text>
        </div>
        <div className="tl-bar" aria-hidden="true">
          {rows.map((r) => (
            <i key={r.name} className={r.k === 'f' ? '' : r.k} />
          ))}
        </div>
      </div>
    );
  }

  return (
    <ol className="col" style={{ gap: 0, margin: 0, padding: 0, listStyle: 'none' }}>
      {rows.map((r, i) => (
        <li key={r.name} className="tl-row">
          <div className="tl-rail">
            <span className={'tl-dot ' + r.k}>{r.sym}</span>
            {i < rows.length - 1 && <span className={'tl-ln' + (i < stage ? ' d' : '')} />}
          </div>
          <div className="grow" style={{ padding: '2px 0 14px' }}>
            <span style={{ fontSize: 17, lineHeight: '24px', ...(i === stage ? { fontWeight: 600 } : i > stage ? { color: 'var(--text2)' } : {}) }}>{r.name}</span>
            {r.cap && <MaxTypography.Text className="cap" variant="detail" color="secondary">{r.cap}</MaxTypography.Text>}
          </div>
        </li>
      ))}
    </ol>
  );
}
