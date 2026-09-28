import { Typography as MaxTypography } from '@maxhub/max-ui';
import { ProgressM2 } from '../../components/ProgressM2';
import { Timeline } from '../../components/Timeline';
import { Badge, Btn, Foot, Header, IconButton, Kv, Main, Note, PollDisclaimer, Screen, Tile, Card } from '../../components/ui';
import { fmtM2 } from '../../lib/format';
import { agenda, initiative } from '../../mocks/demo';
import { P } from '../../paths';

function Title({ withDate }: { withDate?: boolean }) {
  return (
    <div className="col" style={{ gap: 6, padding: '0 4px' }}>
      <MaxTypography.Headline className="h1" variant="large-strong">{initiative.title}</MaxTypography.Headline>
      <MaxTypography.Text className="cap" variant="detail" color="secondary">
        Инициатор: {initiative.initiator}
        {withDate && ` · создана ${initiative.createdAt}`}
      </MaxTypography.Text>
    </div>
  );
}

function About({ withText }: { withText?: boolean }) {
  return (
    <Card className="card" style={{ gap: 10 }}>
      <MaxTypography.Headline className="h3" variant="small">О чём инициатива</MaxTypography.Headline>
      {withText && <MaxTypography.Text className="t" variant="body">{initiative.description}</MaxTypography.Text>}
      <Kv k="Камер">{String(initiative.cameras)}</Kv>
      <Kv k="Оплата">{initiative.payment}</Kv>
      <Kv k="Записи">{initiative.records}</Kv>
    </Card>
  );
}

export function AgendaCard({ badge = true }: { badge?: boolean }) {
  return (
    <Card className="card" style={{ gap: 0 }}>
      <div className="between" style={{ paddingBottom: badge ? 6 : 8 }}>
        <MaxTypography.Headline className="h3" variant="small">Вопросы собрания</MaxTypography.Headline>
        {badge && <Badge kind="calc" />}
      </div>
      {agenda.map((a, i) => (
        <div key={a.short} className="between" style={{ padding: i === agenda.length - 1 ? '10px 0 0' : '10px 0', borderTop: '1px solid var(--line)' }}>
          <span>{a.short}</span>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">{a.rule}</MaxTypography.Text>
        </div>
      ))}
    </Card>
  );
}

const shareBtn = <IconButton icon="share" label="Поделиться" />;

// 7а. Инициатива · Опрос
export function InitPoll() {
  return (
    <Screen>
      <Header title="Инициатива" right={shareBtn} />
      <Main style={{ gap: 14 }}>
        <Title withDate />
        <Card className="card">
          <Timeline stage={1} compact />
        </Card>

        <Card className="card hl" style={{ gap: 14 }}>
          <div className="col" style={{ gap: 2 }}>
            <MaxTypography.Label className="lbl" variant="large-strong">Сейчас</MaxTypography.Label>
            <MaxTypography.Headline className="h3" variant="small">Опрос соседей до {initiative.pollEndsAt}</MaxTypography.Headline>
            <MaxTypography.Text className="t2" variant="body" color="secondary">Узнаём, поддержит ли дом идею, прежде чем звать УК.</MaxTypography.Text>
          </div>
          <ProgressM2 yes={initiative.poll.forM2} voted={initiative.poll.votedM2} title="Ответы в опросе" badge="poll" />
          <PollDisclaimer compact />
        </Card>

        <About withText />
        <AgendaCard />

        <Card className="card">
          <MaxTypography.Headline className="h3" variant="small">Все этапы</MaxTypography.Headline>
          <Timeline
            stage={1}
            caps={[initiative.createdAt, `10 сентября – ${initiative.pollEndsAt} · сейчас`, 'Обязать УК провести собрание', 'Официальное голосование', 'Итог и протокол']}
          />
        </Card>
      </Main>
      <Foot>
        <Btn to={P.vote}>Проголосовать в опросе</Btn>
        <Btn kind="text" to={P.pollProgress}>
          Кто уже ответил
        </Btn>
      </Foot>
    </Screen>
  );
}

// 7б. Инициатива · Требование
export function InitDemand() {
  return (
    <Screen>
      <Header title="Инициатива" right={shareBtn} />
      <Main style={{ gap: 14 }}>
        <Title />
        <Card className="card">
          <Timeline stage={2} compact />
        </Card>

        <Card className="card hl" style={{ gap: 14 }}>
          <div className="col" style={{ gap: 2 }}>
            <MaxTypography.Label className="lbl" variant="large-strong">Сейчас</MaxTypography.Label>
            <MaxTypography.Headline className="h3" variant="small">Требование в УК</MaxTypography.Headline>
          </div>
          <Note kind="pos" style={{ padding: '12px 14px' }}>
            Опрос завершён: «за» <b>{fmtM2(initiative.poll.forM2)}</b> — больше 10%. Можно обязать УК провести собрание.
          </Note>
          <ProgressM2 yes={initiative.poll.forM2} voted={initiative.poll.votedM2} title="Итог опроса" badge="poll" lines="demand" />
          <MaxTypography.Text className="t2" variant="body" color="secondary">Дальше — выбрать, кто проведёт собрание: УК по вашему требованию или вы сами.</MaxTypography.Text>
        </Card>

        <About />
      </Main>
      <Foot>
        <Btn to={P.path}>Выбрать, как провести собрание</Btn>
      </Foot>
    </Screen>
  );
}

// 7в. Инициатива · жилец (не собственник)
export function InitResident() {
  return (
    <Screen>
      <Header title="Инициатива" />
      <Main style={{ gap: 14 }}>
        <Title />
        <Card className="card">
          <Timeline stage={1} compact />
        </Card>
        <Card className="card" style={{ gap: 10 }}>
          <MaxTypography.Headline className="h3" variant="small">О чём инициатива</MaxTypography.Headline>
          <MaxTypography.Text className="t" variant="body">Поставить 6 камер в подъездах. Оплата — 150 ₽ в месяц отдельной строкой в квитанции. Записи хранит УК.</MaxTypography.Text>
        </Card>
        <Card className="card hl" style={{ gap: 12 }}>
          <div className="row" style={{ gap: 12, alignItems: 'flex-start' }}>
            <Tile icon="person" />
            <div className="grow">
              <MaxTypography.Headline className="h3" variant="small">Голосуют только собственники</MaxTypography.Headline>
              <MaxTypography.Text className="t2" variant="body" color="secondary">Вы живёте в кв. 45, но не собственник. Перешлите ссылку тому, кто владеет квартирой, — родственнику или арендодателю.</MaxTypography.Text>
            </div>
          </div>
        </Card>
      </Main>
      <Foot>
        <Btn icon="share">Переслать собственнику</Btn>
      </Foot>
    </Screen>
  );
}
