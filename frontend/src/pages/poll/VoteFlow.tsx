import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Badge, Btn, Foot, Header, Main, Note, Opt, PollDisclaimer, Screen, Status, Card } from '../../components/ui';
import { fmtM2, fmtNum } from '../../lib/format';
import { useApi } from '../../hooks/useApi';
import { fetchInitiative, fetchMe, castVote, parseM2, type VoteInput } from '../../lib/api';

// Страница голосования в опросе
export function VotePage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [choice, setChoice] = useState<'for' | 'against'>('for');
  const [submitting, setSubmitting] = useState(false);

  const initiativeState = useApi(() => (id ? fetchInitiative(id) : Promise.reject('No ID')), [id]);
  const meState = useApi(() => fetchMe());

  const initiative = initiativeState.data;
  const me = meState.data;
  const ownerMemberships = me?.memberships.filter((m) => m.status === 'verified' && m.role === 'owner' && m.house.id === initiative?.house_id) ?? [];

  useEffect(() => {
    if (initiative?.my_vote?.choice === 'for' || initiative?.my_vote?.choice === 'against') setChoice(initiative.my_vote.choice);
  }, [initiative?.my_vote?.choice]);

  const handleSubmit = async () => {
    if (!id || submitting) return;

    setSubmitting(true);
    try {
      const voteData: VoteInput = { choice };
      await castVote(id, voteData);

      // Перенаправляем на страницу анкеты если "за", иначе сразу на страницу успеха
      if (choice === 'for') {
        navigate(`/initiatives/${id}/survey`);
      } else {
        navigate(`/initiatives/${id}/counted`);
      }
    } catch (error) {
      alert(error instanceof Error ? error.message : 'Не удалось отправить голос. Попробуйте ещё раз.');
      setSubmitting(false);
    }
  };

  if (initiativeState.loading || meState.loading) {
    return (
      <Screen>
        <Header title="Опрос" />
        <Main>
          <Card className="card">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text>
          </Card>
        </Main>
      </Screen>
    );
  }

  if (initiativeState.error || meState.error || !initiative || ownerMemberships.length === 0) {
    return (
      <Screen>
        <Header title="Опрос" />
        <Main>
          <Card className="card">
            <Note kind="neg">Не удалось загрузить данные.</Note>
          </Card>
        </Main>
      </Screen>
    );
  }

  const weightM2 = ownerMemberships.reduce((sum, membership) => sum + (parseM2(membership.owner?.weight_m2 ?? null) || 0), 0);
  const areaM2 = ownerMemberships.reduce((sum, membership) => sum + (parseM2(membership.premise.display_area_m2) || 0), 0);
  const totalHouseM2 = parseM2(initiative.total_area_m2) || 0;
  const weightPercent = totalHouseM2 > 0 ? weightM2 / totalHouseM2 * 100 : 0;

  return (
    <Screen>
      <Header title="Опрос" />
      <Main style={{ gap: 14 }}>
        <MaxTypography.Headline className="h2" style={{ padding: '0 4px' }} variant="medium">
          {initiative.title}
        </MaxTypography.Headline>
        <Card className="card" style={{ gap: 8 }}>
          <MaxTypography.Headline className="h3" variant="small">О чём инициатива</MaxTypography.Headline>
          <MaxTypography.Text className="t" variant="body">{initiative.description}</MaxTypography.Text>
        </Card>
        <Card className="card" style={{ flexDirection: 'row', alignItems: 'center', gap: 12 }}>
          <div className="grow">
            <MaxTypography.Text className="cap" variant="detail" color="secondary">Ваш голос</MaxTypography.Text>
            <MaxTypography.Display className="big" style={{ fontSize: 28, lineHeight: '34px' }}>
              {fmtM2(weightM2)}
            </MaxTypography.Display>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">
              {fmtNum(areaM2)} м² суммарно по {ownerMemberships.length} {ownerMemberships.length === 1 ? 'квартире' : 'квартирам'}
            </MaxTypography.Text>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">
              {weightPercent.toLocaleString('ru-RU', { maximumFractionDigits: 3 })}% от общей площади дома
            </MaxTypography.Text>
          </div>
          <Badge kind="calc" style={{ alignSelf: 'center' }} />
        </Card>
        <div className="col" style={{ gap: 10 }} role="radiogroup" aria-label="Ваш ответ">
          <Opt big on={choice === 'for'} onClick={() => setChoice('for')} title="За" />
          <Opt big on={choice === 'against'} onClick={() => setChoice('against')} title="Против" />
        </div>
        <PollDisclaimer />
      </Main>
      <Foot>
        <Btn onClick={handleSubmit} disabled={submitting || !initiative.allowed_actions.some((a) => a.code === 'cast_poll_vote' && a.allowed)}>
          {submitting ? 'Отправка...' : 'Отправить ответ'}
        </Btn>
      </Foot>
    </Screen>
  );
}

// Страница анкеты после голосования "За"
export function SurveyPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const [channel, setChannel] = useState('gosuslugi');
  const [help, setHelp] = useState<string[]>(['chat']);
  const [submitting, setSubmitting] = useState(false);

  const toggleHelp = (v: string) =>
    setHelp((h) => (v === 'no' ? (h.includes('no') ? [] : ['no']) : h.includes(v) ? h.filter((x) => x !== v) : [...h.filter((x) => x !== 'no'), v]));

  const handleSubmit = async () => {
    if (!id || submitting) return;

    setSubmitting(true);
    try {
      // Обновляем голос с анкетой
      await castVote(id, {
        choice: 'for',
        official_channel: channel === 'unknown' ? undefined : (channel as 'gosuslugi' | 'paper'),
        willing_to_help: !help.includes('no'),
      });

      navigate(`/initiatives/${id}/counted`);
    } catch (error) {
      alert(error instanceof Error ? error.message : 'Не удалось сохранить ответы. Попробуйте ещё раз.');
      setSubmitting(false);
    }
  };

  return (
    <Screen>
      <Header title="Ещё два вопроса" right={<MaxTypography.Text className="cap" variant="detail" color="secondary">2 из 2</MaxTypography.Text>} />
      <Main style={{ gap: 14 }}>
        <div className="row" style={{ padding: '0 4px' }}>
          <Status kind="ok">Вы — «за»</Status>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">ответ сохранён</MaxTypography.Text>
        </div>
        <Card className="card" style={{ gap: 10 }}>
          <MaxTypography.Headline className="h3" variant="small">Как вам удобнее голосовать официально?</MaxTypography.Headline>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Настоящее голосование будет позже — на собрании.</MaxTypography.Text>
          <Opt on={channel === 'gosuslugi'} onClick={() => setChannel('gosuslugi')} title="В Госуслуги.Дом" caption="Онлайн, с телефона" />
          <Opt on={channel === 'paper'} onClick={() => setChannel('paper')} title="Бумажный бюллетень" caption="Принесут домой, заберут заполненный" />
          <Opt on={channel === 'unknown'} onClick={() => setChannel('unknown')} title="Пока не знаю" />
        </Card>
        <Card className="card" style={{ gap: 10 }}>
          <MaxTypography.Headline className="h3" variant="small">Готовы помочь собрать голоса?</MaxTypography.Headline>
          <Opt check on={help.includes('floor')} onClick={() => toggleHelp('floor')} title="Обойду соседей на своём этаже" />
          <Opt check on={help.includes('chat')} onClick={() => toggleHelp('chat')} title="Напишу в чат подъезда" />
          <Opt check on={help.includes('no')} onClick={() => toggleHelp('no')} title="Пока не готов(а)" />
        </Card>
      </Main>
      <Foot>
        <Btn onClick={handleSubmit} disabled={submitting}>
          {submitting ? 'Сохранение...' : 'Готово'}
        </Btn>
        <Btn kind="text" onClick={() => navigate(`/initiatives/${id}/counted`)}>
          Пропустить
        </Btn>
      </Foot>
    </Screen>
  );
}

// Страница подтверждения учета голоса
export function CountedPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const initiativeState = useApi(() => (id ? fetchInitiative(id) : Promise.reject('No ID')), [id]);
  const meState = useApi(() => fetchMe());

  const initiative = initiativeState.data;
  const me = meState.data;
  const ownerMemberships = me?.memberships.filter((m) => m.status === 'verified' && m.role === 'owner' && m.house.id === initiative?.house_id) ?? [];

  if (initiativeState.loading || meState.loading) {
    return (
      <Screen>
        <Header nav="close" title="" />
        <Main>
          <Card className="card">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text>
          </Card>
        </Main>
      </Screen>
    );
  }

  if (!initiative || ownerMemberships.length === 0) {
    return (
      <Screen>
        <Header nav="close" title="" />
        <Main>
          <Card className="card">
            <Note kind="neg">Не удалось загрузить данные.</Note>
          </Card>
        </Main>
      </Screen>
    );
  }

  const myVote = initiative.my_vote;
  if (!myVote) return <Screen><Header nav="close" title="" /><Main><Note kind="info">Ваш ответ ещё не зарегистрирован.</Note></Main><Foot><Btn onClick={() => navigate(`/initiatives/${id}/vote`)}>Перейти к голосованию</Btn></Foot></Screen>;
  const weightM2 = myVote.weight_m2 ? parseM2(myVote.weight_m2) || 0 : ownerMemberships.reduce((sum, membership) => sum + (parseM2(membership.owner?.weight_m2 ?? null) || 0), 0);
  const totalHouseM2 = parseM2(initiative.total_area_m2) || 0;
  const weightPercent = totalHouseM2 > 0 ? weightM2 / totalHouseM2 * 100 : 0;
  const choice = myVote?.choice === 'for' ? '«за»' : '«против»';

  return (
    <Screen>
      <Header nav="close" title="" />
      <Main style={{ gap: 16, paddingTop: 16 }}>
        <div className="col" style={{ gap: 8, alignItems: 'center', textAlign: 'center' }}>
          <span className="circ" style={{ background: 'var(--pos-soft)', color: 'var(--pos)', width: 72, height: 72, fontSize: 32 }}>
            ✓
          </span>
          <MaxTypography.Headline className="h2" variant="medium">Голос учтён</MaxTypography.Headline>
          <MaxTypography.Text className="t2" variant="body" color="secondary">
            Вы — {choice}. Ваш голос в опросе: {fmtM2(weightM2)} м² ({weightPercent.toLocaleString('ru-RU', { maximumFractionDigits: 3 })}% площади дома).
          </MaxTypography.Text>
        </div>
        <Card className="card counted-initiative-card" style={{ gap: 12 }}>
          <MaxTypography.Headline className="h3" variant="small">{initiative.title}</MaxTypography.Headline>
          <Status kind="acc">Опрос</Status>
        </Card>
        <Note kind="info">Это опрос, а не голосование собрания. Официально голосовать нужно будет позже — мы напомним.</Note>
      </Main>
      <Foot>
        <Btn onClick={() => navigate(`/initiatives/${id}`)}>Вернуться к инициативе</Btn>
      </Foot>
    </Screen>
  );
}
