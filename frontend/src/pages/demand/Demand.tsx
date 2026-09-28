import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Badge, Btn, Cell, Foot, Header, Kv, Main, Note, Screen, Seg, Status, Steps, Tile, UiButton, UiInput, UiList, Card } from '../../components/ui';
import { Icon } from '../../components/Icon';
import { fmtM2 } from '../../lib/format';
import { house, initiative } from '../../mocks/demo';
import { P } from '../../paths';

// 10. Выбор пути: A — через УК, B — самостоятельно
export function PathChoice() {
  const navigate = useNavigate();
  const [path, setPath] = useState<'A' | 'B'>('A');
  const pathStyle = (on: boolean) => ({ gap: 10, boxShadow: on ? 'inset 0 0 0 2px var(--acc)' : 'inset 0 0 0 1.5px var(--line)' });

  return (
    <Screen>
      <Header title="Кто проведёт собрание" />
      <Main style={{ gap: 14 }}>
        <MaxTypography.Text className="t" style={{ padding: '0 4px' }} variant="body">
          Опрос показал поддержку. Теперь нужно официальное собрание — выберите, кто его проведёт.
        </MaxTypography.Text>
        <UiButton type="button" className="path-card" variant="secondary" style={pathStyle(path === 'A')} onClick={() => setPath('A')} aria-pressed={path === 'A'}>
          <div className="between">
            <Status kind="acc">Рекомендуем</Status>
            <span className={'rd' + (path === 'A' ? ' on' : '')} />
          </div>
          <MaxTypography.Headline className="h3" variant="small">A. Через УК</MaxTypography.Headline>
          <MaxTypography.Text className="t2" variant="body" color="secondary">Вы передаёте требование — УК обязана провести собрание за 45 дней: уведомит соседей, раздаст бюллетени, посчитает голоса.</MaxTypography.Text>
          <div className="row" style={{ gap: 8, paddingTop: 2 }}>
            <Status kind="ok">Доступно</Status>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">
              {fmtM2(initiative.poll.forM2)} «за» — больше 10% ({fmtM2(house.thresholds.demandM2)})
            </MaxTypography.Text>
          </div>
        </UiButton>
        <UiButton type="button" className="path-card" variant="secondary" style={pathStyle(path === 'B')} onClick={() => setPath('B')} aria-pressed={path === 'B'}>
          <div className="between">
            <MaxTypography.Headline className="h3" variant="small">B. Провести самостоятельно</MaxTypography.Headline>
            <span className={'rd' + (path === 'B' ? ' on' : '')} />
          </div>
          <MaxTypography.Text className="t2" variant="body" color="secondary">Вы сами уведомляете соседей, собираете бюллетени и оформляете протокол. Быстрее, но больше работы.</MaxTypography.Text>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Приложение подскажет каждый шаг</MaxTypography.Text>
        </UiButton>
      </Main>
      <Foot>
        <Btn onClick={() => navigate(path === 'A' ? P.demand : P.ukCreateMeeting)}>{path === 'A' ? 'Продолжить через УК' : 'Провести самостоятельно'}</Btn>
      </Foot>
    </Screen>
  );
}

// 11. Требование в УК
export function Demand() {
  const [channel, setChannel] = useState<'personal' | 'mail' | 'gis'>('personal');
  const [date, setDate] = useState('2026-10-01');

  return (
    <Screen>
      <Header title="Требование в УК" />
      <Main style={{ gap: 14 }}>
        <Card className="card" style={{ flexDirection: 'row', gap: 12, alignItems: 'center' }}>
          <Tile icon="receipt" style={{ width: 52, height: 60 }} />
          <div className="grow">
            <span style={{ fontWeight: 600 }}>Требование о проведении общего собрания</span>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">{house.uk} · 2 страницы</MaxTypography.Text>
          </div>
        </Card>
        <Btn kind="secondary" icon="download">
          Скачать PDF
        </Btn>

        <div className="field">
          <MaxTypography.Label className="lbl" style={{ padding: '0 4px' }} variant="large-strong">
            Как передать
          </MaxTypography.Label>
          <Seg
            value={channel}
            onChange={setChannel}
            options={[
              { value: 'personal', label: 'Лично' },
              { value: 'mail', label: 'Почтой' },
              { value: 'gis', label: 'ГИС ЖКХ' },
            ]}
          />
        </div>

        <Card className="card" style={{ gap: 14 }}>
          <MaxTypography.Headline className="h3" variant="small">Что сделать</MaxTypography.Headline>
          <Steps
            items={[
              <>
                Распечатайте PDF. Под ним должны подписаться собственники — вместе <b>не меньше {fmtM2(house.thresholds.demandM2)}</b> (10% дома).
              </>,
              'Сделайте копию и отнесите оба экземпляра в офис УК.',
              <>
                Попросите поставить на вашей копии <b>отметку о приёме с датой</b>. С этой даты идут 45 дней.
              </>,
              'Вернитесь сюда и отметьте, что передали.',
            ]}
          />
        </Card>

        <Card className="card" style={{ gap: 8 }}>
          <div className="between">
            <MaxTypography.Headline className="h3" variant="small">Кто подписал</MaxTypography.Headline>
            <Badge kind="calc" />
          </div>
          <Kv k="Собственников «за» в опросе">26 кв.</Kv>
          <Kv k="Их площадь">{`${fmtM2(initiative.poll.forM2)} · 41%`}</Kv>
          <Kv k="Нужно для требования">{`${fmtM2(house.thresholds.demandM2)} · ~6 кв.`}</Kv>
        </Card>

        <label className="field">
          <MaxTypography.Label className="lbl" style={{ padding: '0 4px' }} variant="large-strong">
            Дата передачи
          </MaxTypography.Label>
          <UiInput type="date" value={date} onChange={(e) => setDate(e.target.value)} iconBefore={<Icon name="calendar" className="chev" />} />
        </label>
      </Main>
      <Foot>
        <Btn to={P.countdown}>Отметить, что передано</Btn>
      </Foot>
    </Screen>
  );
}

// 11б. Отсчёт 45 дней
export function Countdown() {
  const passed = 7;
  return (
    <Screen>
      <Header title="Требование в УК" />
      <Main style={{ gap: 14 }}>
        <Card className="card" style={{ gap: 12 }}>
          <div className="between">
            <Status kind="acc">Передано 1 октября</Status>
            <Badge kind="calc" />
          </div>
          <div className="row" style={{ alignItems: 'baseline', gap: 10 }}>
            <MaxTypography.Display className="big">{45 - passed} дней</MaxTypography.Display>
            <MaxTypography.Text className="t2" variant="body" color="secondary">осталось у УК</MaxTypography.Text>
          </div>
          <div className="pb-track" style={{ marginTop: 4 }}>
            <div className="pb-yes" style={{ width: (passed / 45) * 100 + '%' }} />
          </div>
          <div className="between">
            <MaxTypography.Text className="cap" variant="detail" color="secondary">прошло {passed} из 45</MaxTypography.Text>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">до 15 ноября 2026</MaxTypography.Text>
          </div>
          <MaxTypography.Text className="t2" variant="body" color="secondary">За это время УК должна назначить собрание и уведомить всех собственников.</MaxTypography.Text>
        </Card>
        <Card className="card" style={{ gap: 0 }}>
          <MaxTypography.Headline className="h3" style={{ paddingBottom: 8 }} variant="small">
            События
          </MaxTypography.Headline>
          <div className="row" style={{ gap: 12, padding: '10px 0', borderTop: '1px solid var(--line)' }}>
            <span className="tl-dot d">✓</span>
            <div className="grow">
              <span>Требование передано лично</span>
              <MaxTypography.Text className="cap" variant="detail" color="secondary">1 октября · отметка о приёме есть</MaxTypography.Text>
            </div>
          </div>
          <div className="row" style={{ gap: 12, padding: '10px 0 0', borderTop: '1px solid var(--line)' }}>
            <span className="tl-dot f">…</span>
            <div className="grow">
              <span style={{ color: 'var(--text2)' }}>Ждём: УК назначит собрание</span>
              <MaxTypography.Text className="cap" variant="detail" color="secondary">пришлём сообщение в MAX</MaxTypography.Text>
            </div>
          </div>
        </Card>
        <Note kind="info">45 дней — срок по Жилищному кодексу (ст. 45), считается с даты отметки о приёме.</Note>
      </Main>
      <Foot>
        <Btn icon="share">Рассказать в чате дома</Btn>
      </Foot>
    </Screen>
  );
}

// 11в. Просрочка
export function Overdue() {
  return (
    <Screen>
      <Header title="Требование в УК" />
      <Main style={{ gap: 14 }}>
        <Card className="card" style={{ gap: 12 }}>
          <div className="between">
            <Status kind="bad">Просрочено на 3 дня</Status>
            <Badge kind="calc" />
          </div>
          <MaxTypography.Headline className="h2" variant="medium">УК не провела собрание в срок</MaxTypography.Headline>
          <div className="pb-track" style={{ marginTop: 0 }}>
            <div className="pb-yes" style={{ width: '100%', background: 'var(--neg)' }} />
          </div>
          <div className="between">
            <MaxTypography.Text className="cap" variant="detail" color="secondary">передано 1 октября</MaxTypography.Text>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">срок истёк 15 ноября</MaxTypography.Text>
          </div>
        </Card>
        <MaxTypography.Text className="t" style={{ padding: '0 4px' }} variant="body">
          Это нарушение, но решение всё ещё можно принять. Есть два пути:
        </MaxTypography.Text>
        <UiList>
          <Cell style={{ minHeight: 84 }} lead={<Tile icon="personPlus" />} chevron to={P.ukCreateMeeting}>
            <MaxTypography.Headline className="h3" variant="small">Провести самостоятельно</MaxTypography.Headline>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Путь B — приложение проведёт по шагам</MaxTypography.Text>
          </Cell>
          <Cell style={{ minHeight: 84 }} lead={<Tile icon="docAlert" style={{ background: 'var(--neg-soft)', color: 'var(--neg)' }} />} chevron onClick={() => {}}>
            <MaxTypography.Headline className="h3" variant="small">Жалоба в жилищную инспекцию</MaxTypography.Headline>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Подготовим текст с датами и копией требования</MaxTypography.Text>
          </Cell>
        </UiList>
        <MaxTypography.Text className="cap" style={{ padding: '0 4px' }} variant="detail" color="secondary">
          Можно и то и другое: жалоба не мешает провести собрание самим.
        </MaxTypography.Text>
      </Main>
      <Foot>
        <Btn to={P.ukCreateMeeting}>Провести самостоятельно</Btn>
        <Btn kind="text">Подготовить жалобу</Btn>
      </Foot>
    </Screen>
  );
}
