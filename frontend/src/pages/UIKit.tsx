import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { Badge, Btn, Cell, Chip, Circle, DemoBadge, Header, IconButton, Kv, LinkBtn, Main, Note, Opt, Screen, Seg, StateHead, Status, Steps, Tile, Apt, UiList, Card, useTheme } from '../components/ui';
import { Icon, type IconName } from '../components/Icon';
import { ProgressM2 } from '../components/ProgressM2';
import { Timeline } from '../components/Timeline';

const allIcons: IconName[] = [
  'back',
  'chevron',
  'close',
  'check',
  'plus',
  'minus',
  'share',
  'search',
  'qr',
  'scan',
  'phone',
  'receipt',
  'doc',
  'docAlert',
  'zip',
  'people',
  'person',
  'personX',
  'personPlus',
  'info',
  'clock',
  'alert',
  'lock',
  'calendar',
  'download',
  'external',
  'edit',
  'video',
  'intercom',
  'barrier',
  'building',
  'house',
  'more',
  'offline',
  'chat',
  'retry',
  'bulb',
];

export function UIKit() {
  const { theme, changeTheme } = useTheme();
  const [selected, setSelected] = useState('opt1');
  const [checks, setChecks] = useState({ a: true, b: false, c: false });
  const [segment, setSegment] = useState<'one' | 'two' | 'three'>('one');
  const [chips, setChips] = useState({ c1: true, c2: false, c3: false });
  const [listMessage, setListMessage] = useState('');

  return (
    <Screen>
      <Header title="UI Kit" nav={null} />
      <Main className="ui-kit-main" style={{ gap: 24, paddingBottom: 100 }}>
        <div className="ui-kit-theme-control">
          <span className="ui-kit-theme-label">Тема оформления</span>
          <Seg
            options={[{ value: 'light', label: 'Светлая' }, { value: 'dark', label: 'Тёмная' }]}
            value={theme}
            onChange={changeTheme}
          />
        </div>
        {/* Section: Icons */}
        <Section title="Icons">
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(60px, 1fr))', gap: 12 }}>
            {allIcons.map((name) => (
              <div key={name} style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 4 }}>
                <Icon name={name} />
                <span style={{ fontSize: 11, color: 'var(--text2)', textAlign: 'center' }}>{name}</span>
              </div>
            ))}
          </div>
        </Section>

        {/* Section: Buttons */}
        <Section title="Buttons">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Btn>Primary Button</Btn>
            <Btn icon="plus">With Icon</Btn>
            <Btn iconRight="chevron">With Right Icon</Btn>
            <Btn kind="secondary">Secondary</Btn>
            <Btn kind="text">Text Button</Btn>
            <Btn kind="negative">Negative</Btn>
            <Btn disabled>Disabled</Btn>
            <Btn small>Small Button</Btn>
            <LinkBtn icon="share">Link Button</LinkBtn>
            <IconButton icon="close" label="Close" />
          </div>
        </Section>

        {/* Section: Badges & Status */}
        <Section title="Badges & Status">
          <div className="row wrap" style={{ gap: 8 }}>
            <Badge kind="fact" />
            <Badge kind="calc" />
            <Badge kind="poll" />
            <Badge kind="model" />
            <DemoBadge />
          </div>
          <div className="row wrap" style={{ gap: 8, marginTop: 12 }}>
            <Status kind="ok">Собственник</Status>
            <Status kind="acc">Опрос</Status>
            <Status kind="paper">Бумага</Status>
            <Status kind="said">Со слов</Status>
            <Status kind="none">Гость</Status>
            <Status kind="bad">Недейств.</Status>
            <Status kind="mod">Скрыто</Status>
          </div>
        </Section>

        {/* Section: Notes */}
        <Section title="Notes">
          <Note kind="info">Информация для пользователя</Note>
          <Note kind="poll">Это опрос, а не голосование собрания</Note>
          <Note kind="pos" icon="check">
            Положительное сообщение
          </Note>
          <Note kind="neg" icon="alert">
            Предупреждение или ошибка
          </Note>
        </Section>

        {/* Section: Circles */}
        <Section title="Circle Icons">
          <div className="row" style={{ gap: 12 }}>
            <Circle icon="bulb" tone="acc" />
            <Circle icon="clock" tone="warn" />
            <Circle icon="close" tone="neg" />
            <Circle icon="check" tone="pos" />
          </div>
        </Section>

        {/* Section: State Heads */}
        <Section title="State Heads">
          <StateHead icon="bulb" tone="acc" title="Заголовок состояния" text="Описание ситуации в 1-2 предложениях" />
        </Section>

        {/* Section: Cells & Lists */}
        <Section title="Cells & Lists">
          <UiList>
            <Cell>
              <strong>Информация о доме</strong>
              <MaxTypography.Text variant="detail" color="secondary">Статичная строка с описанием</MaxTypography.Text>
            </Cell>
            <Cell chevron onClick={() => setListMessage('Это пример действия по нажатию на строку списка.')}>
              <strong>Настройки уведомлений</strong>
              <MaxTypography.Text variant="detail" color="secondary">Нажатие показывает подсказку</MaxTypography.Text>
            </Cell>
            <Cell lead={<Tile icon="phone" />} trail={<Status kind="ok">На связи</Status>}>
              <strong>Контакты УК</strong>
              <MaxTypography.Text variant="detail" color="secondary">Иконка и статус справа</MaxTypography.Text>
            </Cell>
            <Cell lead={<Apt n="45" />}>
              <strong>Кв. 45 · 52,3 м²</strong>
              <MaxTypography.Text className="cap" variant="detail" color="secondary">Подъезд 2 · этаж 5</MaxTypography.Text>
            </Cell>
          </UiList>
          {listMessage && <Note kind="info">{listMessage}</Note>}
        </Section>

        {/* Section: Key-Value Pairs */}
        <Section title="Key-Value Pairs">
          <Card className="card">
            <Kv k="Площадь">52,3 м²</Kv>
            <Kv k="Доля">1/2</Kv>
            <Kv k="Вес голоса">
              <span className="row">
                <b style={{ fontSize: 18 }}>26,15 м²</b>
                <Badge kind="calc" />
              </span>
            </Kv>
          </Card>
        </Section>

        {/* Section: Options (Radio & Checkbox) */}
        <Section title="Options (Radio & Checkbox)">
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            <Opt on={selected === 'opt1'} onClick={() => setSelected('opt1')} title="Первый вариант" caption="Пояснение к первому варианту" />
            <Opt on={selected === 'opt2'} onClick={() => setSelected('opt2')} title="Второй вариант" caption="Пояснение ко второму" />
            <Opt on={selected === 'opt3'} onClick={() => setSelected('opt3')} title="Третий вариант" />
          </div>
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8, marginTop: 16 }}>
            <Opt check on={checks.a} onClick={() => setChecks({ ...checks, a: !checks.a })} title="Checkbox A" />
            <Opt check on={checks.b} onClick={() => setChecks({ ...checks, b: !checks.b })} title="Checkbox B" />
            <Opt check on={checks.c} onClick={() => setChecks({ ...checks, c: !checks.c })} title="Checkbox C" />
          </div>
        </Section>

        {/* Section: Segmented Control */}
        <Section title="Segmented Control">
          <Seg
            options={[
              { value: 'one', label: 'One' },
              { value: 'two', label: 'Two' },
              { value: 'three', label: 'Three' },
            ]}
            value={segment}
            onChange={setSegment}
          />
        </Section>

        {/* Section: Chips */}
        <Section title="Chips">
          <div className="row wrap" style={{ gap: 8 }}>
            <Chip on={chips.c1} onClick={() => setChips({ ...chips, c1: !chips.c1 })}>
              Chip One
            </Chip>
            <Chip on={chips.c2} onClick={() => setChips({ ...chips, c2: !chips.c2 })} icon="check">
              With Icon
            </Chip>
            <Chip on={chips.c3} onClick={() => setChips({ ...chips, c3: !chips.c3 })}>
              Chip Three
            </Chip>
          </div>
        </Section>

        {/* Section: Steps */}
        <Section title="Steps">
          <Card className="card">
            <Steps
              items={[
                'Распечатайте бюллетень и принесите УК',
                'УК проверит подпись и внесёт ваш голос',
                'Следите за прогрессом в приложении',
              ]}
            />
          </Card>
        </Section>

        {/* Section: Progress Bar */}
        <Section title="Progress Bar (ProgressM2)">
          <Card className="card">
            <ProgressM2 yes={1240} voted={1420} lines="all" />
          </Card>
          <Card className="card" style={{ marginTop: 12 }}>
            <ProgressM2 yes={850} voted={850} compact title="" cap="850 м² «за» из 2000 нужных" />
          </Card>
        </Section>

        {/* Section: Timeline */}
        <Section title="Timeline">
          <Card className="card">
            <Timeline stage={1} caps={['Создана 15 сен', 'Идёт до 5 окт', '', '', '']} />
          </Card>
          <Card className="card" style={{ marginTop: 12 }}>
            <Timeline stage={2} compact />
          </Card>
          <Card className="card" style={{ marginTop: 12 }}>
            <Timeline stage={4} />
          </Card>
          <Card className="card" style={{ marginTop: 12 }}>
            <Timeline stage={1} cancelled />
          </Card>
        </Section>

        {/* Section: Cards */}
        <Section title="Cards">
          <Card className="card">
            <MaxTypography.Headline className="h3" variant="small">Card Title</MaxTypography.Headline>
            <MaxTypography.Text className="t" variant="body">This is a card with some content inside. Cards have padding, background, and rounded corners.</MaxTypography.Text>
          </Card>
          <Card className="card" style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
            <span className="av">А</span>
            <div className="grow">
              <MaxTypography.Headline className="h3" variant="small">Анна · кв. 45</MaxTypography.Headline>
              <MaxTypography.Text className="cap" variant="detail" color="secondary">Вес голоса 26,15 м²</MaxTypography.Text>
            </div>
          </Card>
        </Section>

        {/* Section: Typography */}
        <Section title="Typography">
          <MaxTypography.Headline className="h1" variant="large-strong">Heading 1 (h1)</MaxTypography.Headline>
          <MaxTypography.Headline className="h2" variant="medium">Heading 2 (h2)</MaxTypography.Headline>
          <MaxTypography.Headline className="h3" variant="small">Heading 3 (h3)</MaxTypography.Headline>
          <MaxTypography.Text className="t" variant="body">Regular text (t)</MaxTypography.Text>
          <MaxTypography.Text className="t2" variant="body" color="secondary">Secondary text (t2)</MaxTypography.Text>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Caption text (cap)</MaxTypography.Text>
          <MaxTypography.Label className="lbl" variant="large-strong">Label text (lbl)</MaxTypography.Label>
          <MaxTypography.Display className="big">Big number (big)</MaxTypography.Display>
          <br />
          <span className="num">Number (num)</span>
        </Section>

        {/* Section: Utilities */}
        <Section title="Utilities">
          <div className="row" style={{ gap: 12 }}>
            <Apt n="45" />
            <Apt n="12" size={32} />
            <Tile icon="phone" />
            <span className="av">А</span>
            <span className="av" style={{ background: 'linear-gradient(155deg,#bf97ff 6%,#526eff 84%)' }}>
              Д
            </span>
          </div>
          <div className="hr" style={{ margin: '16px 0' }} />
          <div className="between">
            <span>Between layout</span>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Right side</MaxTypography.Text>
          </div>
        </Section>
      </Main>
    </Screen>
  );
}

function Section({ title, children }: { title: string; children: React.ReactNode }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
      <MaxTypography.Headline className="h2" style={{ padding: '0 4px', color: 'var(--acc-t)' }} variant="medium">
        {title}
      </MaxTypography.Headline>
      {children}
    </div>
  );
}
