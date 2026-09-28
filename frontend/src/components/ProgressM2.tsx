import { Typography as MaxTypography } from '@maxhub/max-ui';
import { house } from '../mocks/demo';
import { approxFlats, fmtM2, fmtNum, pct } from '../lib/format';
import { Badge, type BadgeKind } from './ui';

// Полоса прогресса в м² с метками 10% / кворум / 2/3 и строками «сколько не хватает».
// Перенос компонента ProgressM2 из дизайна.

type Lines = 'all' | 'demand' | 'quorum' | 'twothirds' | 'none';

type Props = {
  /** м² «за» (или «проголосовали» — см. yesLabel) */
  yes: number;
  /** м² всех ответивших, «за» + «против» */
  voted?: number;
  title?: string;
  badge?: BadgeKind;
  lines?: Lines;
  compact?: boolean;
  yesLabel?: string;
  othLabel?: string;
  /** подпись под компактной полосой */
  cap?: string;
  totalM2?: number;
  thresholds?: { demandM2: number; quorumAboveM2: number; twoThirdsM2: number };
  avgAreaM2?: number;
};

type Line = { ok: boolean; text: string; result: string };

export function ProgressM2({
  yes,
  voted: votedProp,
  title = 'Поддержка в опросе',
  badge = 'poll',
  lines: linesProp,
  compact = false,
  yesLabel = '«за»',
  othLabel = '«против»',
  cap,
  totalM2 = house.totalAreaM2,
  thresholds = house.thresholds,
  avgAreaM2 = house.avgAreaM2,
}: Props) {
  const voted = Math.max(votedProp ?? yes, yes);
  const mode: Lines = linesProp ?? (compact ? 'none' : 'all');
  const { demandM2, quorumAboveM2, twoThirdsM2 } = thresholds;
  const kv = (m2: number) => approxFlats(m2, avgAreaM2);

  const lines: Line[] = [];
  if (mode === 'all' || mode === 'demand') {
    const text = `10% · ${fmtM2(demandM2)} — можно обязать УК провести собрание`;
    lines.push(
      yes >= demandM2
        ? { ok: true, text, result: `Есть: ${fmtM2(yes)} «за»` }
        : { ok: false, text, result: `Не хватает ${fmtM2(demandM2 - yes)} · ~${kv(demandM2 - yes)} кв.` },
    );
  }
  if (mode === 'all' || mode === 'quorum') {
    const text = `Кворум — участвуют больше ${fmtM2(quorumAboveM2)}`;
    lines.push(
      voted > quorumAboveM2
        ? { ok: true, text, result: `Есть: ${fmtM2(voted)}` }
        : { ok: false, text, result: `Нужно ещё больше ${fmtM2(quorumAboveM2 - voted)} · ~${kv(quorumAboveM2 - voted + 0.01)} кв.` },
    );
  }
  if (mode === 'all' || mode === 'twothirds') {
    const text = `2/3 «за» · ${fmtM2(twoThirdsM2)} — решение принято`;
    lines.push(
      yes >= twoThirdsM2
        ? { ok: true, text, result: `Есть: ${fmtM2(yes)} «за»` }
        : { ok: false, text, result: `Не хватает ${fmtM2(twoThirdsM2 - yes)} · ~${kv(twoThirdsM2 - yes)} кв.` },
    );
  }

  const yp = Math.min((yes / totalM2) * 100, 100);
  const op = Math.min(((voted - yes) / totalM2) * 100, 100 - yp);
  const marks = [
    { x: (demandM2 / totalM2) * 100, label: '10%' },
    { x: (quorumAboveM2 / totalM2) * 100, label: '>50%' },
    { x: (twoThirdsM2 / totalM2) * 100, label: '2/3' },
  ];

  return (
    <div className="pb">
      {title !== '' && (
        <div className="between">
          <MaxTypography.Headline className="h3" variant="small">{title}</MaxTypography.Headline>
          <Badge kind={badge} />
        </div>
      )}
      {!compact && (
        <div className="row wrap" style={{ alignItems: 'baseline', gap: 8 }}>
          <MaxTypography.Display className="big">{fmtM2(yes)}</MaxTypography.Display>
          <MaxTypography.Text className="t2" variant="body" color="secondary">
            {yesLabel} · {pct(yes, totalM2)}% площади дома
          </MaxTypography.Text>
        </div>
      )}
      <div
        className={compact ? 'pb-track c' : 'pb-track'}
        role="img"
        aria-label={`${fmtNum(yes)} м² ${yesLabel} из ${fmtNum(totalM2)} м² площади дома`}
      >
        <div className="pb-yes" style={{ width: yp + '%', borderRadius: op > 0 ? '99px 0 0 99px' : undefined }} />
        <div className="pb-oth" style={{ left: yp + '%', width: op + '%' }} />
        {marks.map((m) => (
          <div key={m.label} className="pb-mk" style={{ left: m.x + '%' }}>
            <span>{compact ? '' : m.label}</span>
          </div>
        ))}
      </div>
      {!compact && voted > yes && (
        <div className="row wrap" style={{ gap: 16 }}>
          <span className="lg">
            <i style={{ background: 'var(--acc)' }} />
            {yesLabel} {fmtM2(yes)}
          </span>
          <span className="lg">
            <i style={{ background: 'var(--bar2)' }} />
            {othLabel} {fmtM2(voted - yes)}
          </span>
        </div>
      )}
      {compact && <MaxTypography.Text className="cap" variant="detail" color="secondary">{cap ?? `${fmtM2(yes)} ${yesLabel} из ${fmtM2(twoThirdsM2)} нужных`}</MaxTypography.Text>}
      {lines.map((l) => (
        <div key={l.text} className="pb-line">
          <span className={'pb-ic ' + (l.ok ? 'ok' : 'no')}>{l.ok ? '✓' : ''}</span>
          <div className="grow">
            <span style={{ fontSize: 16, lineHeight: '22px' }}>{l.text}</span>
            <span style={{ fontSize: 16, lineHeight: '22px', fontWeight: 600, color: l.ok ? 'var(--pos)' : 'var(--text)' }}>{l.result}</span>
          </div>
        </div>
      ))}
    </div>
  );
}
