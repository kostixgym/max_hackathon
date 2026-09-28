import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { ProgressM2 } from '../../components/ProgressM2';
import { Timeline } from '../../components/Timeline';
import { Badge, Btn, Foot, Header, IconButton, Kv, Main, Note, PollDisclaimer, Screen, Status, Card } from '../../components/ui';
import { fmtM2 } from '../../lib/format';
import { useApi } from '../../hooks/useApi';
import { fetchInitiative, fetchInitiativeDemand, fetchInitiativeMeeting, fetchMe, fetchPollProgress, parseM2, startPoll } from '../../lib/api';

function DemandLink({ initiativeId }: { initiativeId: string }) {
  const navigate = useNavigate();
  const state = useApi(() => fetchInitiativeDemand(initiativeId), [initiativeId]);
  if (state.loading) return <Note kind="info">Загружаем требование…</Note>;
  if (state.error || !state.data) return <Note kind="neg">Не удалось открыть требование. Попробуйте обновить страницу.</Note>;
  return <Btn onClick={() => navigate(`/demands/${state.data!.id}`)}>Открыть требование в УК</Btn>;
}

function MeetingLink({ initiativeId }: { initiativeId: string }) {
  const navigate = useNavigate();
  const state = useApi(() => fetchInitiativeMeeting(initiativeId), [initiativeId]);
  if (state.loading) return <Note kind="info">Загружаем собрание…</Note>;
  if (state.error || !state.data) return <Note kind="neg">Не удалось открыть собрание. Попробуйте обновить страницу.</Note>;
  return <Btn onClick={() => navigate(`/meetings/${state.data!.id}`)}>Открыть собрание</Btn>;
}

function AgendaCard({ items, badge = true }: { items: { position: number; text: string; majority_rule: string }[]; badge?: boolean }) {
  return (
    <Card className="card" style={{ gap: 0 }}>
      <div className="between" style={{ paddingBottom: badge ? 6 : 8 }}>
        <MaxTypography.Headline className="h3" variant="small">Вопросы собрания</MaxTypography.Headline>
        {badge && <Badge kind="calc" />}
      </div>
      {items.map((a, i) => (
        <div key={a.position} className="between" style={{ padding: i === items.length - 1 ? '10px 0 0' : '10px 0', borderTop: '1px solid var(--line)' }}>
          <span>{a.text}</span>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">{a.majority_rule}</MaxTypography.Text>
        </div>
      ))}
    </Card>
  );
}

function Title({ title, initiator, createdAt, withDate }: { title: string; initiator?: string; createdAt: string; withDate?: boolean }) {
  const date = new Date(createdAt).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
  return (
    <div className="col" style={{ gap: 6, padding: '0 4px' }}>
      <MaxTypography.Headline className="h1" variant="large-strong">{title}</MaxTypography.Headline>
      <MaxTypography.Text className="cap" variant="detail" color="secondary">
        {initiator && `Инициатор: ${initiator}`}
        {withDate && ` · создана ${date}`}
      </MaxTypography.Text>
    </div>
  );
}

function About({ description, params }: { description: string; params: any }) {
  const accessLabels: Record<string, string> = { management_company: 'Управляющая компания', contractor: 'Обслуживающий подрядчик', house_council: 'Совет дома' };
  return (
    <Card className="card" style={{ gap: 10 }}>
      <MaxTypography.Headline className="h3" variant="small">О чём инициатива</MaxTypography.Headline>
      <MaxTypography.Text className="t" variant="body">{description}</MaxTypography.Text>
      {params && typeof params === 'object' && Object.keys(params).length > 0 && (
        <>
          {params.placement && <Kv k="Где устанавливаем">{String(params.placement)}</Kv>}
          {params.camera_count != null && <Kv k="Камер">{String(params.camera_count)}</Kv>}
          {params.estimated_cost_rub != null && <Kv k="Ориентир стоимости">{new Intl.NumberFormat('ru-RU').format(Number(params.estimated_cost_rub))} ₽</Kv>}
          {params.payment_method && <Kv k="Оплата">{params.payment_method === 'management_bill' ? 'Строкой в квитанции УК' : 'Разовым целевым сбором'}</Kv>}
          {params.records_access && <Kv k="Доступ к записям">{accessLabels[String(params.records_access)] ?? String(params.records_access)}</Kv>}
        </>
      )}
    </Card>
  );
}

function ShareButton({ id }: { id: string }) {
  const share = async () => {
    const url = `${window.location.origin}${window.location.pathname}#/initiatives/${id}`;
    try {
      if (navigator.share) await navigator.share({ url });
      else if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(url);
        window.alert('Ссылка на инициативу скопирована');
      } else window.prompt('Скопируйте ссылку на инициативу', url);
    } catch {
      // Закрытие системного окна «Поделиться» не требует сообщения об ошибке.
    }
  };
  return <IconButton icon="share" label="Поделиться инициативой" onClick={share} />;
}

// Универсальная страница инициативы
export function InitiativeView() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const initiativeState = useApi(() => (id ? fetchInitiative(id) : Promise.reject('No ID')), [id]);
  const meState = useApi(() => fetchMe());
  const pollState = useApi(() => (id && (initiativeState.data?.stage === 'poll' || initiativeState.data?.stage === 'demand') ? fetchPollProgress(id) : Promise.reject('No poll')), [id, initiativeState.data?.stage]);
  const [startingPoll, setStartingPoll] = useState(false);
  const homePath = (() => {
    const house = initiativeState.data && meState.data?.memberships.find((membership) => membership.house.id === initiativeState.data?.house_id)?.house;
    return house ? `/home?house=${encodeURIComponent(house.slug)}` : '/';
  })();

  const handleStartPoll = async () => {
    if (!id || startingPoll) return;
    setStartingPoll(true);
    try {
      await startPoll(id);
      window.location.reload();
    } catch (e) {
      window.alert(e instanceof Error ? e.message : 'Не удалось запустить опрос');
      setStartingPoll(false);
    }
  };

  if (initiativeState.loading) {
    return (
      <Screen>
        <Header title="Инициатива" />
        <Main>
          <Card className="card">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text>
          </Card>
        </Main>
      </Screen>
    );
  }

  if (initiativeState.error || !initiativeState.data || !id) {
    return (
      <Screen>
        <Header title="Инициатива" />
        <Main>
          <Card className="card">
            <Note kind="neg">Не удалось загрузить инициативу. Попробуйте обновить страницу.</Note>
          </Card>
        </Main>
      </Screen>
    );
  }

  const initiative = initiativeState.data;
  const poll = pollState.data;
  const canStartPoll = initiative.allowed_actions.some((a) => a.code === 'start_poll' && a.allowed);
  const totalAreaM2 = parseM2(initiative.total_area_m2) ?? 0;
  const thresholds = {
    demandM2: parseM2(initiative.thresholds.demand_m2) ?? 0,
    quorumAboveM2: parseM2(initiative.thresholds.quorum_above_m2) ?? 0,
    twoThirdsM2: parseM2(initiative.thresholds.two_thirds_m2) ?? 0,
  };

  if ((initiative.stage === 'poll' || initiative.stage === 'demand') && (pollState.loading || pollState.error || !poll)) {
    return <Screen><Header title="Инициатива" /><Main>{pollState.loading ? <Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка прогресса…</MaxTypography.Text></Card> : <Note kind="neg">Не удалось загрузить опрос. Проверьте доступ к дому и попробуйте обновить страницу.</Note>}</Main></Screen>;
  }
  const canVote = initiative.allowed_actions.some((a) => a.code === 'cast_poll_vote' && a.allowed);

  // Poll stage
  if (initiative.stage === 'poll' && poll) {
    const pollEndsAt = initiative.poll_ends_at ? new Date(initiative.poll_ends_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' }) : '';
    const forM2 = parseM2(poll.for_m2) || 0;
    const votedM2 = (parseM2(poll.against_m2) || 0) + forM2;

    return (
      <Screen>
        <Header title="Инициатива" onNav={() => navigate(homePath)} right={<ShareButton id={id} />} />
        <Main style={{ gap: 14 }}>
          <Title title={initiative.title} createdAt={initiative.created_at} withDate />
          <Card className="card">
            <Timeline stage={1} compact />
          </Card>

          <Card className="card hl" style={{ gap: 14 }}>
            <div className="col" style={{ gap: 2 }}>
              <MaxTypography.Label className="lbl" variant="large-strong">Сейчас</MaxTypography.Label>
              <MaxTypography.Headline className="h3" variant="small">Опрос соседей до {pollEndsAt}</MaxTypography.Headline>
              <MaxTypography.Text className="t2" variant="body" color="secondary">Узнаём, поддержит ли дом идею, прежде чем звать УК.</MaxTypography.Text>
            </div>
            <ProgressM2 yes={forM2} voted={votedM2} totalM2={totalAreaM2} thresholds={thresholds} title="Ответы в опросе" badge="poll" />
            <PollDisclaimer compact />
          </Card>

          <About description={initiative.description} params={initiative.params} />
          <AgendaCard items={initiative.agenda_items} />

          <Card className="card">
            <MaxTypography.Headline className="h3" variant="small">Все этапы</MaxTypography.Headline>
            <Timeline
              stage={1}
              caps={[
                new Date(initiative.created_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' }),
                `Опрос до ${pollEndsAt} · сейчас`,
                'Обязать УК провести собрание',
                'Официальное голосование',
                'Итог и протокол',
              ]}
            />
          </Card>
        </Main>
        <Foot>
          {canVote ? <Btn onClick={() => navigate(`/initiatives/${id}/vote`)}>{initiative.my_vote ? 'Изменить ответ' : 'Проголосовать в опросе'}</Btn> : <Note kind="info">Голосовать могут подтверждённые собственники, пока опрос открыт.</Note>}
          {initiative.is_initiator && poll.demand_reached && <Btn kind="secondary" onClick={() => navigate(`/initiatives/${id}/path`)}>Выбрать путь к собранию</Btn>}
          <Btn kind="text" onClick={() => navigate(`/initiatives/${id}/progress`)}>
            Кто уже ответил
          </Btn>
        </Foot>
      </Screen>
    );
  }

  // Demand stage
  if (initiative.stage === 'demand' && poll) {
    const forM2 = parseM2(poll.for_m2) || 0;
    const votedM2 = (parseM2(poll.against_m2) || 0) + forM2;

    return (
      <Screen>
        <Header title="Инициатива" onNav={() => navigate(homePath)} right={<ShareButton id={id} />} />
        <Main style={{ gap: 14 }}>
          <Title title={initiative.title} createdAt={initiative.created_at} />
          <Card className="card">
            <Timeline stage={2} compact />
          </Card>

          <Card className="card hl" style={{ gap: 14 }}>
            <div className="col" style={{ gap: 2 }}>
              <MaxTypography.Label className="lbl" variant="large-strong">Сейчас</MaxTypography.Label>
              <MaxTypography.Headline className="h3" variant="small">Требование в УК</MaxTypography.Headline>
            </div>
            <Note kind="pos" style={{ padding: '12px 14px' }}>
              Опрос завершён: «за» <b>{fmtM2(forM2)}</b> — больше 10%. Можно обязать УК провести собрание.
            </Note>
            <ProgressM2 yes={forM2} voted={votedM2} totalM2={totalAreaM2} thresholds={thresholds} title="Итог опроса" badge="poll" lines="demand" />
            <MaxTypography.Text className="t2" variant="body" color="secondary">Дальше — выбрать, кто проведёт собрание: УК по вашему требованию или вы сами.</MaxTypography.Text>
          </Card>

          <About description={initiative.description} params={initiative.params} />
        </Main>
        <Foot>
          <DemandLink initiativeId={id} />
        </Foot>
      </Screen>
    );
  }

  // Default: show basic info
  return (
    <Screen>
      <Header title="Инициатива" onNav={() => navigate(homePath)} right={<ShareButton id={id} />} />
      <Main style={{ gap: 14 }}>
        <Title title={initiative.title} createdAt={initiative.created_at} withDate />
        <About description={initiative.description} params={initiative.params} />
        {initiative.agenda_items.length > 0 && <AgendaCard items={initiative.agenda_items} />}
        {initiative.is_initiator && initiative.stage === 'draft' && <Note kind="info">Проверьте формулировку и параметры перед запуском опроса соседей.</Note>}
      </Main>
      {initiative.is_initiator && canStartPoll && <Foot><Btn onClick={handleStartPoll} disabled={startingPoll}>{startingPoll ? 'Запускаем…' : 'Запустить опрос'}</Btn></Foot>}
      {(initiative.stage === 'meeting' || initiative.stage === 'done') && <Foot><MeetingLink initiativeId={id} /></Foot>}
    </Screen>
  );
}

export function PollProgressPage() {
  const { id } = useParams<{ id: string }>();
  const initiativeState = useApi(() => (id ? fetchInitiative(id) : Promise.reject(new Error('Нет ID инициативы'))), [id]);
  const pollState = useApi(() => (id ? fetchPollProgress(id) : Promise.reject(new Error('Нет ID инициативы'))), [id]);
  if (initiativeState.loading || pollState.loading) return <Screen><Header title="Ход опроса" /><Main><Card className="card"><MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка…</MaxTypography.Text></Card></Main></Screen>;
  if (initiativeState.error || pollState.error || !initiativeState.data || !pollState.data) return <Screen><Header title="Ход опроса" /><Main><Note kind="neg">Не удалось загрузить прогресс опроса.</Note></Main></Screen>;

  const poll = pollState.data;
  const yes = parseM2(poll.for_m2) ?? 0;
  const no = parseM2(poll.against_m2) ?? 0;
  const threshold = parseM2(poll.demand_m2) ?? 0;
  const totalArea = parseM2(poll.total_m2) ?? 0;
  const houseThresholds = {
    demandM2: threshold,
    quorumAboveM2: parseM2(poll.thresholds.quorum_above_m2) ?? 0,
    twoThirdsM2: parseM2(poll.thresholds.two_thirds_m2) ?? 0,
  };
  const ends = poll.poll_ends_at ? new Date(poll.poll_ends_at).toLocaleString('ru-RU', { day: 'numeric', month: 'long', hour: '2-digit', minute: '2-digit' }) : '—';

  return <Screen>
    <Header title="Ход опроса" />
    <Main style={{ gap: 14 }}>
      <MaxTypography.Headline className="h2" style={{ padding: '0 4px' }} variant="medium">{initiativeState.data.title}</MaxTypography.Headline>
      <Card className="card" style={{ gap: 14 }}>
        <ProgressM2 yes={yes} voted={yes + no} totalM2={totalArea} thresholds={houseThresholds} title="Ответы в опросе" badge="poll" />
        <div className="hr" />
        <div className="between"><span>Поддерживают</span><b>{fmtM2(yes)} м² · {poll.votes_for} голосов</b></div>
        <div className="between"><span>Против</span><b>{fmtM2(no)} м² · {poll.votes_against} голосов</b></div>
        <div className="between"><span>Порог для требования</span><b>{fmtM2(threshold)} м²</b></div>
        <Status kind={poll.demand_reached ? 'ok' : 'acc'}>{poll.demand_reached ? 'Порог достигнут' : `Опрос до ${ends}`}</Status>
      </Card>
      <Note kind="info">Backend отдаёт общий прогресс без списка проголосовавших квартир. Имена и номера квартир здесь не показываются.</Note>
    </Main>
  </Screen>;
}
