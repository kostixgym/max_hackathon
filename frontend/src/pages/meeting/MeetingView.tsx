import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useParams, useNavigate } from 'react-router-dom';
import { ProgressM2 } from '../../components/ProgressM2';
import { Badge, Btn, Foot, Header, Kv, Main, Note, Screen, Status, UiList, Card } from '../../components/ui';
import { fmtM2 } from '../../lib/format';
import { useApi } from '../../hooks/useApi';
import { fetchMeeting, fetchMeetingTracker, parseM2 } from '../../lib/api';

function AgendaCard({ items }: { items: { position: number; text: string; majority_rule: string }[] }) {
  return (
    <Card className="card" style={{ gap: 0 }}>
      <div className="between" style={{ paddingBottom: 8 }}>
        <MaxTypography.Headline className="h3" variant="small">Вопросы собрания</MaxTypography.Headline>
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

function getStatusLabel(status: string): string {
  switch (status) {
    case 'notice':
      return 'Уведомление';
    case 'voting':
      return 'Идёт голосование';
    case 'counting':
      return 'Подсчёт';
    case 'finalized':
      return 'Завершено';
    default:
      return status;
  }
}

function getFormLabel(form: string): string {
  switch (form) {
    case 'gis_electronic':
      return 'Электронное, Госуслуги.Дом';
    case 'paper_absentee':
      return 'Заочное на бумаге';
    default:
      return form;
  }
}

// Карточка собрания
export function MeetingView() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const meetingState = useApi(() => (id ? fetchMeeting(id) : Promise.reject('No ID')), [id]);

  if (meetingState.loading) {
    return (
      <Screen>
        <Header title="Собрание" />
        <Main>
          <Card className="card">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text>
          </Card>
        </Main>
      </Screen>
    );
  }

  if (meetingState.error || !meetingState.data || !id) {
    return (
      <Screen>
        <Header title="Собрание" />
        <Main>
          <Card className="card">
            <Note kind="neg">Не удалось загрузить собрание. Попробуйте обновить страницу.</Note>
          </Card>
        </Main>
      </Screen>
    );
  }

  const meeting = meetingState.data;
  const participantsM2 = parseM2(meeting.progress.participants_m2) || 0;
  const totalM2 = parseM2(meeting.progress.total_m2) || 0;

  const noticeAt = new Date(meeting.notice_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
  const votingStartsAt = new Date(meeting.voting_starts_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });
  const votingEndsAt = new Date(meeting.voting_ends_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' });

  const daysLeft = Math.ceil((new Date(meeting.voting_ends_at).getTime() - Date.now()) / (1000 * 60 * 60 * 24));

  return (
    <Screen>
      <Header title="Собрание" />
      <Main style={{ gap: 14 }}>
        <div className="col" style={{ gap: 8, padding: '0 4px' }}>
          <Status kind="acc" style={{ alignSelf: 'flex-start' }}>
            {getStatusLabel(meeting.status)}
          </Status>
          <MaxTypography.Headline className="h1" variant="large-strong">{meeting.title}</MaxTypography.Headline>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">Собрание №{meeting.attempt} · {meeting.house.address}</MaxTypography.Text>
        </div>

        <Card className="card" style={{ gap: 10 }}>
          <div className="between">
            <MaxTypography.Headline className="h3" variant="small">Даты</MaxTypography.Headline>
            <Badge kind="fact" />
          </div>
          <Kv k="Форма">{getFormLabel(meeting.form)}</Kv>
          <Kv k="Уведомление">{noticeAt}</Kv>
          <Kv k="Голосование">
            {votingStartsAt} – {votingEndsAt}
          </Kv>
          {daysLeft > 0 && meeting.status === 'voting' && <Kv k="Осталось">{daysLeft} дней</Kv>}
        </Card>

        <Card className="card" style={{ gap: 14 }}>
          <ProgressM2 yes={participantsM2} voted={totalM2} title="Учтено голосов" badge="calc" lines="quorum" yesLabel="проголосовали" />
          <div className="hr" />
          <div className="between">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Бюллетени получены</MaxTypography.Text>
            <MaxTypography.Text className="cap" variant="detail" color="secondary">
              {meeting.progress.ballots_received} из {meeting.progress.ballots_total}
            </MaxTypography.Text>
          </div>
        </Card>

        <AgendaCard items={meeting.agenda_items} />

        {meeting.is_admin && (
          <Card className="card" style={{ gap: 10 }}>
            <div className="between">
              <MaxTypography.Headline className="h3" variant="small">Администратор</MaxTypography.Headline>
              <Badge kind="fact" />
            </div>
            <Kv k="Председатель">{meeting.chair.masked_name}</Kv>
            <Kv k="Секретарь">{meeting.secretary.masked_name}</Kv>
          </Card>
        )}
      </Main>
      <Foot>
        {meeting.is_admin && meeting.status === 'voting' && (
          <>
            <Btn onClick={() => navigate(`/meetings/${id}/tracker`)}>Трекер квартир</Btn>
            <Btn kind="text" onClick={() => navigate(`/meetings/${id}/result-preview`)}>
              Предпросмотр итога
            </Btn>
          </>
        )}
        {meeting.status === 'finalized' && (
          <Btn icon="download" onClick={() => window.open(`/api/v1/meetings/${id}/protocol.pdf`, '_blank')}>
            Скачать протокол
          </Btn>
        )}
      </Foot>
    </Screen>
  );
}

// Трекер квартир (для администратора)
export function TrackerView() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();

  const trackerState = useApi(() => (id ? fetchMeetingTracker(id) : Promise.reject('No ID')), [id]);

  if (trackerState.loading) {
    return (
      <Screen>
        <Header title="Трекер квартир" />
        <Main>
          <Card className="card">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text>
          </Card>
        </Main>
      </Screen>
    );
  }

  if (trackerState.error || !trackerState.data || !id) {
    return (
      <Screen>
        <Header title="Трекер квартир" />
        <Main>
          <Card className="card">
            <Note kind="neg">Не удалось загрузить трекер. Попробуйте обновить страницу.</Note>
          </Card>
        </Main>
      </Screen>
    );
  }

  const tracker = trackerState.data;
  const participantsM2 = parseM2(tracker.summary.participants_m2) || 0;

  const statusLabels: Record<string, { kind: 'ok' | 'paper' | 'said' | 'bad' | 'none'; label: string }> = {
    counted: { kind: 'ok', label: 'Учтён' },
    received: { kind: 'paper', label: 'Получен' },
    pending: { kind: 'none', label: 'Не получен' },
  };

  return (
    <Screen>
      <Header title="Трекер квартир" />
      <Main style={{ gap: 12 }}>
        <Card className="card" style={{ gap: 10, padding: '14px 16px' }}>
          <div className="between">
            <MaxTypography.Headline className="h3" variant="small">Сводка</MaxTypography.Headline>
            <Badge kind="calc" />
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(2, minmax(0, 1fr))', gap: '8px 12px' }}>
            <span className="row">
              <Status kind="ok">Учтено</Status>
              <b className="num">{tracker.summary.counted}</b>
            </span>
            <span className="row">
              <Status kind="paper">Получено</Status>
              <b className="num">{tracker.summary.received}</b>
            </span>
            <span className="row">
              <Status kind="none">Не получено</Status>
              <b className="num">{tracker.summary.ballots_total - tracker.summary.received}</b>
            </span>
          </div>
          <div className="hr" />
          <div className="between">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Участников</MaxTypography.Text>
            <b>{fmtM2(participantsM2)}</b>
          </div>
        </Card>

        <UiList>
          {tracker.ballots.map((ballot) => {
            const status = statusLabels[ballot.status] || { kind: 'none', label: ballot.status };
            const weightM2 = parseM2(ballot.weight_m2) || 0;
            return (
              <div key={ballot.id} className="cell" style={{ minHeight: 60 }}>
                <div className="grow">
                  <span style={{ fontWeight: 600 }}>
                    Кв. {ballot.premise_number} · {fmtM2(weightM2)}
                  </span>
                  <MaxTypography.Text className="cap" variant="detail" color="secondary">
                    {ballot.entrance && `Подъезд ${ballot.entrance} · `}
                    {ballot.owner_masked_name}
                  </MaxTypography.Text>
                </div>
                <Status kind={status.kind}>{status.label}</Status>
              </div>
            );
          })}
        </UiList>
      </Main>
      <Foot>
        <Btn onClick={() => navigate(`/meetings/${id}/receive`)}>Принять бюллетень</Btn>
      </Foot>
    </Screen>
  );
}
