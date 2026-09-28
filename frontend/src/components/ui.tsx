import { createContext, useContext, type ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { Avatar as MaxAvatar, Button as MaxButton, CellAction as MaxCellAction, CellList as MaxCellList, CellSimple as MaxCellSimple, Flex as MaxFlex, IconButton as MaxIconButton, Input as MaxInput, Panel as MaxPanel, Spinner as MaxSpinner, Typography as MaxTypography } from '@maxhub/max-ui';
import { Icon, type IconName } from './Icon';

// ---------- Тема ----------

export type AppTheme = 'light' | 'dark';
export const ThemeContext = createContext<{ theme: AppTheme; changeTheme: (theme: AppTheme) => void }>({
  theme: 'light',
  changeTheme: () => {},
});
export const useTheme = () => useContext(ThemeContext);

// ---------- Каркас экрана ----------

/** Корень экрана (.scr). Тёмная тема — класс .t-dark из maxkit.css. */
export function Screen({ children, dark }: { children: ReactNode; dark?: boolean }) {
  const { theme } = useTheme();
  const isDark = dark ?? theme === 'dark';
  return <MaxPanel mode="primary" className={'scr' + (isDark ? ' t-dark' : '')}>{children}</MaxPanel>;
}

type HeaderProps = {
  title?: ReactNode;
  /** 'back' — стрелка назад, 'close' — крестик, null — без кнопки */
  nav?: 'back' | 'close' | null;
  onNav?: () => void;
  right?: ReactNode;
};

export function Header({ title, nav = 'back', onNav, right }: HeaderProps) {
  const navigate = useNavigate();
  const go = onNav ?? (() => navigate(-1));
  return (
    <div className="hdr">
      {nav && (
        <MaxIconButton type="button" className="max-icon-button" aria-label={nav === 'back' ? 'Назад' : 'Закрыть'} onClick={go} variant="ghost" size="large">
          <Icon name={nav === 'back' ? 'back' : 'close'} />
        </MaxIconButton>
      )}
      <div className="ttl"><MaxTypography.Title variant="large-strong">{title}</MaxTypography.Title></div>
      {right}
    </div>
  );
}

export function IconButton({ icon, label, onClick }: { icon: IconName; label: string; onClick?: () => void }) {
  return (
    <MaxIconButton type="button" className="max-icon-button" aria-label={label} onClick={onClick} variant="ghost" size="large">
      <Icon name={icon} />
    </MaxIconButton>
  );
}

export function Main({ children, style, className }: { children: ReactNode; style?: React.CSSProperties; className?: string }) {
  return (
    <MaxFlex className={'main' + (className ? ' ' + className : '')} direction="column" style={style}>
      {children}
    </MaxFlex>
  );
}

export function Foot({ children }: { children: ReactNode }) {
  return <MaxPanel mode="secondary" className="foot">{children}</MaxPanel>;
}

export function Card({ children, className, ...props }: React.ComponentProps<typeof MaxPanel>) {
  return <MaxPanel mode="secondary" className={className} {...props}>{children}</MaxPanel>;
}

// ---------- Кнопки ----------

type BtnKind = 'primary' | 'secondary' | 'text' | 'negative';

type BtnProps = {
  children: ReactNode;
  kind?: BtnKind;
  small?: boolean;
  icon?: IconName;
  iconRight?: IconName;
  to?: string;
  onClick?: () => void;
  disabled?: boolean;
  style?: React.CSSProperties;
};

export function Btn({ children, kind = 'primary', small, icon, iconRight, to, onClick, disabled, style }: BtnProps) {
  const navigate = useNavigate();
  const handle = onClick ?? (to ? () => navigate(to) : undefined);
  const variant = kind === 'primary' ? 'primary' : kind === 'secondary' ? 'secondary' : kind === 'negative' ? 'destructive' : 'ghost';
  return (
    <MaxButton type="button" className="max-app-button" variant={variant} size={small ? 'small' : 'large'} stretched onClick={handle} disabled={disabled} style={style}
      iconBefore={icon ? <Icon name={icon} small={kind === 'text'} /> : undefined}
      iconAfter={iconRight ? <Icon name={iconRight} small /> : undefined}>
      {children}
    </MaxButton>
  );
}

export function LinkBtn({ children, icon, onClick, to, style }: { children: ReactNode; icon?: IconName; onClick?: () => void; to?: string; style?: React.CSSProperties }) {
  const navigate = useNavigate();
  return (
    <MaxButton type="button" className="max-link-button" variant="ghost" size="medium" onClick={onClick ?? (to ? () => navigate(to) : undefined)} style={style}>
      {icon && <Icon name={icon} small />}
      {children}
    </MaxButton>
  );
}

// ---------- Бейджи и статусы ----------

export type BadgeKind = 'fact' | 'calc' | 'poll' | 'model';
const badgeText: Record<BadgeKind, string> = { fact: 'Факт', calc: 'Расчёт', poll: 'Опрос — без юр. силы', model: 'Модельные данные' };
const badgeDescription: Record<BadgeKind, string> = { fact: 'Данные из реестра дома', calc: 'Расчёт по площади и долям квартир', poll: 'Предварительный опрос без юридической силы', model: 'Демо-данные для примера' };

/** Бейдж источника данных: факт / расчёт / опрос / модельные данные. */
export function Badge({ kind, children, style }: { kind: BadgeKind; children?: ReactNode; style?: React.CSSProperties }) {
  return (
    <MaxTypography.Label variant="medium-strong" className={'b b-' + kind} style={style} title={badgeDescription[kind]} aria-label={badgeDescription[kind]}>
      {children ?? badgeText[kind]}
    </MaxTypography.Label>
  );
}

export function DemoBadge({ children = 'Демо-дом' }: { children?: ReactNode }) {
  return <MaxTypography.Label variant="medium-strong" className="max-demo-badge">{children}</MaxTypography.Label>;
}

export type StatusKind = 'none' | 'said' | 'paper' | 'ok' | 'bad' | 'acc' | 'mod';

export function Status({ kind, children, style }: { kind: StatusKind; children: ReactNode; style?: React.CSSProperties }) {
  return <MaxTypography.Label variant="medium" className={'max-status max-status-' + kind} style={style}>{children}</MaxTypography.Label>;
}

// ---------- Плашки ----------

type NoteKind = 'poll' | 'info' | 'neg' | 'pos' | 'mod';
const noteIcon: Record<NoteKind, IconName> = { poll: 'info', info: 'info', neg: 'alert', pos: 'check', mod: 'info' };

export function Note({ kind, icon, children, style }: { kind: NoteKind; icon?: IconName; children: ReactNode; style?: React.CSSProperties }) {
  return (
    <MaxPanel mode="secondary" className={'note n-' + kind} style={style}>
      <Icon name={icon ?? noteIcon[kind]} />
      <MaxTypography.Text variant="body">{children}</MaxTypography.Text>
    </MaxPanel>
  );
}

/** Стандартная плашка «опрос без юридической силы». */
export function PollDisclaimer({ children, compact }: { children?: ReactNode; compact?: boolean }) {
  return (
    <Note kind="poll" style={compact ? { padding: '12px 14px', fontSize: 15, lineHeight: '21px' } : undefined}>
      {children ?? 'Это опрос, а не голосование собрания. Юридической силы не имеет.'}
    </Note>
  );
}

// ---------- Строки и списки ----------

export function Kv({ k, children }: { k: ReactNode; children: ReactNode }) {
  return (
    <div className="kv">
      <span>{k}</span>
      {typeof children === 'string' ? <b>{children}</b> : children}
    </div>
  );
}

/** Нумерованные шаги: «1 · Распечатайте PDF…». */
export function Steps({ items }: { items: ReactNode[] }) {
  return (
    <>
      {items.map((item, i) => (
        <div key={i} className="row" style={{ alignItems: 'flex-start', gap: 12 }}>
          <span className="step">{i + 1}</span>
          <MaxTypography.Text className="t" variant="body">{item}</MaxTypography.Text>
        </div>
      ))}
    </>
  );
}

type CellProps = {
  children: ReactNode;
  lead?: ReactNode;
  trail?: ReactNode;
  chevron?: boolean;
  to?: string;
  onClick?: () => void;
  style?: React.CSSProperties;
};

/** Ячейка списка (.cell). Кликабельная — если задан to или onClick. */
export function Cell({ children, lead, trail, chevron, to, onClick, style }: CellProps) {
  const navigate = useNavigate();
  const handle = onClick ?? (to ? () => navigate(to) : undefined);
  const content = <div className="max-cell-copy">{children}</div>;
  if (handle) return <MaxCellAction type="button" mode="secondary" className="max-cell" before={lead} showChevron={chevron} onClick={handle} style={style}><div className="max-cell-body">{content}{trail && <span className="max-cell-trail">{trail}</span>}</div></MaxCellAction>;
  return <MaxCellSimple className="max-cell" title={content} before={lead} after={trail} style={style} />;
}

export function Tile({ icon, style }: { icon: IconName; style?: React.CSSProperties }) {
  return (
    <MaxPanel mode="secondary" className="tile" style={style}>
      <Icon name={icon} />
    </MaxPanel>
  );
}

export function Apt({ n, size }: { n: ReactNode; size?: number }) {
  return (
    <MaxPanel mode="secondary" className="apt" style={size ? { width: size, height: size, fontSize: size < 48 ? 15 : undefined } : undefined}>
      <MaxTypography.Title variant="medium-strong">{n}</MaxTypography.Title>
    </MaxPanel>
  );
}

// ---------- Экран-состояние (иконка в круге + заголовок + текст) ----------

type Tone = 'acc' | 'warn' | 'neg' | 'pos';
const toneStyle: Record<Tone, React.CSSProperties> = {
  acc: { background: 'var(--acc-soft)', color: 'var(--acc-t)' },
  warn: { background: 'var(--warn-soft)', color: 'var(--warn)' },
  neg: { background: 'var(--neg-soft)', color: 'var(--neg)' },
  pos: { background: 'var(--pos-soft)', color: 'var(--pos)' },
};

export function Circle({ icon, tone, size }: { icon: IconName; tone: Tone; size?: number }) {
  return (
    <MaxPanel mode="secondary" className="circ" style={{ ...toneStyle[tone], ...(size ? { width: size, height: size } : {}) }}>
      <Icon name={icon} />
    </MaxPanel>
  );
}

export function StateHead({ icon, tone, title, text }: { icon: IconName; tone: Tone; title: ReactNode; text?: ReactNode }) {
  return (
    <>
      <Circle icon={icon} tone={tone} />
      <MaxTypography.Headline variant="medium" className="max-state-title">
        {title}
      </MaxTypography.Headline>
      {text && (
        <MaxTypography.Text variant="body" color="secondary" className="max-state-description">
          {text}
        </MaxTypography.Text>
      )}
    </>
  );
}

// ---------- Выбор ----------

type OptProps = {
  on: boolean;
  onClick: () => void;
  title: ReactNode;
  caption?: ReactNode;
  check?: boolean;
  big?: boolean;
};

/** Вариант выбора: радио (по умолчанию) или чекбокс. */
export function Opt({ on, onClick, title, caption, check, big }: OptProps) {
  return (
    <MaxCellAction type="button" className={'max-option' + (on ? ' selected' : '')} onClick={onClick} aria-pressed={on}
      before={<span className={'max-choice' + (check ? ' checkbox' : ' radio') + (on ? ' selected' : '')}>{on && (check ? <Icon name="check" style={{ width: 16, height: 16, strokeWidth: 3 }} /> : <span className="max-choice-radio-dot" />)}</span>}>
      <span className="max-option-content"><MaxTypography.Title variant={big ? 'medium-strong' : 'small-strong'}>{title}</MaxTypography.Title>{caption && <MaxTypography.Text variant="detail" color="secondary">{caption}</MaxTypography.Text>}</span>
    </MaxCellAction>
  );
}

export function Seg<T extends string>({ options, value, onChange }: { options: { value: T; label: ReactNode }[]; value: T; onChange: (v: T) => void }) {
  return (
    <div className="seg">
      {options.map((o) => (
        <MaxButton key={o.value} type="button" className="max-segment" variant={o.value === value ? 'secondary' : 'ghost'} size="medium" onClick={() => onChange(o.value)} aria-pressed={o.value === value}>
          {o.label}
        </MaxButton>
      ))}
    </div>
  );
}

export function Chip({ on, onClick, children, icon, style }: { on: boolean; onClick: () => void; children: ReactNode; icon?: IconName; style?: React.CSSProperties }) {
  return (
    <MaxButton type="button" className="max-chip" variant={on ? 'primary' : 'secondary'} size="small" onClick={onClick} aria-pressed={on} style={style}
      iconBefore={icon ? <Icon name={icon} small /> : undefined}>{children}</MaxButton>
  );
}

export function UiInput(props: React.ComponentProps<typeof MaxInput>) {
  return <MaxInput className={'max-app-input ' + (props.className ?? '')} size="large" {...props} />;
}

export function UiButton({ variant = 'secondary', size = 'medium', className, ...props }: React.ComponentProps<typeof MaxButton>) {
  return <MaxButton variant={variant} size={size} className={'max-app-button ' + (className ?? '')} {...props} />;
}

export function UiList({ children, className, ...props }: React.ComponentProps<typeof MaxCellList>) {
  return <MaxCellList mode="island" className={'max-app-list ' + (className ?? '')} {...props}>{children}</MaxCellList>;
}

export function UiSpinner() {
  return <MaxSpinner size={24} />;
}

export function UiAvatar({ src, name }: { src?: string; name: string }) {
  return <MaxAvatar.Container size={56} form="circle" className="max-app-avatar">
    {src ? <MaxAvatar.Image src={src} alt="" /> : <MaxAvatar.Text>{name.slice(0, 1).toUpperCase()}</MaxAvatar.Text>}
  </MaxAvatar.Container>;
}
