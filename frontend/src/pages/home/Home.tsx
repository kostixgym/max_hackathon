import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Apt, Badge, Btn, Cell, DemoBadge, Foot, Header, Kv, Main, Note, Screen, Status, UiButton, type StatusKind, UiList, Card } from '../../components/ui';
import { fmtM2, fmtNum } from '../../lib/format';
import { P } from '../../paths';
import { useApi } from '../../hooks/useApi';
import { fetchMe, fetchHouse, fetchInitiatives, getStartParam, parseM2 } from '../../lib/api';

function HomeHeader({ address, isDemo }: { address: string; isDemo: boolean }) {
  const navigate = useNavigate();
  return (
    <>
      <Header title="Мой дом" onNav={() => navigate('/')} right={isDemo ? <DemoBadge /> : undefined} />
      <div className="home-address"><MaxTypography.Text className="cap" variant="detail" color="secondary">{address}</MaxTypography.Text></div>
    </>
  );
}

function getStageStatus(stage: string): StatusKind {
  switch (stage) {
    case 'poll':
      return 'acc';
    case 'demand':
      return 'acc';
    case 'meeting':
      return 'acc';
    case 'done':
      return 'ok';
    case 'cancelled':
      return 'none';
    default:
      return 'none';
  }
}

function getStageLabel(stage: string): string {
  switch (stage) {
    case 'draft':
      return 'Черновик';
    case 'poll':
      return 'Опрос';
    case 'demand':
      return 'Требование';
    case 'meeting':
      return 'Собрание';
    case 'done':
      return 'Завершена';
    case 'cancelled':
      return 'Отменена';
    default:
      return stage;
  }
}

// 4. Мой дом — собственник
export function Home() {
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const meState = useApi(() => fetchMe());
  const me = meState.data;

  // Берем первую подтвержденную привязку как owner
  const linkedSlug = searchParams.get('house') ?? me?.house?.slug ?? getStartParam();
  const selectedOwners = me?.memberships.filter((m) => m.status === 'verified' && m.role === 'owner' && (!linkedSlug || m.house.slug === linkedSlug)) ?? [];
  const ownerMemberships = selectedOwners.length ? selectedOwners : (me?.memberships.filter((m) => m.status === 'verified' && m.role === 'owner') ?? []);
  const ownerMembership = ownerMemberships[0];
  const house = ownerMembership?.house;
  const houseId = house?.id;
  const houseState = useApi(() => house ? fetchHouse(house.slug) : Promise.resolve(null), [house?.slug]);

  const initiativesState = useApi(() => (houseId ? fetchInitiatives(houseId) : Promise.resolve({ initiatives: [] })), [houseId]);

  if (meState.loading || initiativesState.loading || houseState.loading) {
    return (
      <Screen>
        <Main>
          <Card className="card">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text>
          </Card>
        </Main>
      </Screen>
    );
  }

  if (meState.error || initiativesState.error || houseState.error || !me || !ownerMembership || !house) {
    return (
      <Screen>
        <Main>
          <Card className="card">
            <Note kind="neg">Не удалось загрузить данные. Попробуйте обновить страницу.</Note>
          </Card>
        </Main>
      </Screen>
    );
  }

  const memberships = ownerMemberships.filter((m) => m.house.id === house.id);
  const totalArea = memberships.reduce((sum, m) => sum + (parseM2(m.premise.display_area_m2) || 0), 0);
  const totalWeight = memberships.reduce((sum, m) => sum + (parseM2(m.owner?.weight_m2 ?? null) || 0), 0);
  const homeTotalArea = parseM2(houseState.data?.total_area_m2 ?? null) || 0;
  const homeWeightPercent = homeTotalArea > 0 ? totalWeight / homeTotalArea * 100 : 0;
  const initiatives = initiativesState.data?.initiatives || [];
  const activeInitiatives = initiatives.filter((i) => i.stage === 'draft' || i.stage === 'poll' || i.stage === 'demand' || i.stage === 'meeting');
  const archivedInitiatives = initiatives.filter((i) => i.stage === 'done' || i.stage === 'cancelled');
  return (
    <Screen>
      <HomeHeader address={house.address} isDemo={house.is_demo} />
      <Main>
        <Card className="card home-profile-card">
          <div className="row" style={{ gap: 12 }}>
            <span className="av">{me.user.first_name[0]}</span>
            <div className="grow">
              <MaxTypography.Headline className="h3" variant="small">
                {me.user.first_name} · {memberships.length} {memberships.length === 1 ? 'квартира' : 'квартиры'}
              </MaxTypography.Headline>
              <div className="row">
                <Status kind="ok">Собственник</Status>
                {activeInitiatives.some((i) => i.is_initiator) && <Status kind="acc">Инициатор</Status>}
              </div>
            </div>
          </div>
          <div className="hr" />
          <Kv k="Квартиры">
            <span className="row"><b>{memberships.map((m) => m.premise.number).join(', ')}</b></span>
          </Kv>
          <Kv k="Площадь">
            <span className="row">
              <b>{fmtM2(totalArea)}</b>
              <Badge kind="fact" />
            </span>
          </Kv>
          <Kv k="Ваш вес голоса">
            <span className="row"><b>{fmtM2(totalWeight)} м² · {homeWeightPercent.toLocaleString('ru-RU', { maximumFractionDigits: 3 })}% дома</b><Badge kind="calc" /></span>
          </Kv>
          {memberships.length > 0 && (
            <>
              <Kv k="Вес голоса">
                <span className="row">
                  <b style={{ fontSize: 18 }}>{fmtM2(totalWeight)}</b>
                  <Badge kind="calc" />
                </span>
              </Kv>
            </>
          )}
        </Card>

        <div className="between" style={{ padding: '6px 4px 0' }}>
          <MaxTypography.Headline className="h3" variant="small">Инициативы дома</MaxTypography.Headline>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">{initiatives.length}</MaxTypography.Text>
        </div>

        {activeInitiatives.map((init) => (
          <UiButton
            key={init.id}
            type="button"
            className="initiative-card"
            variant="secondary"
            onClick={() => navigate(`/initiatives/${init.id}`)}
            style={{ gap: 10, justifyContent: 'center' }}
          >
            <div className="between" style={{ alignItems: 'flex-start' }}>
              <MaxTypography.Headline className="h3" variant="small">{init.title}</MaxTypography.Headline>
              <Status kind={getStageStatus(init.stage)}>{getStageLabel(init.stage)}</Status>
            </div>
            {init.stage === 'poll' && init.poll_ends_at && (
              <div className="between">
                <MaxTypography.Text className="cap" variant="detail" color="secondary">Опрос до {new Date(init.poll_ends_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long' })}</MaxTypography.Text>
                <Badge kind="poll">Опрос</Badge>
              </div>
            )}
          </UiButton>
        ))}

        {archivedInitiatives.length > 0 && (
          <UiList>
            {archivedInitiatives.map((init) => (
              <Cell key={init.id} onClick={() => navigate(`/initiatives/${init.id}`)} trail={<Status kind={getStageStatus(init.stage)}>{getStageLabel(init.stage)}</Status>}>
                <span style={{ fontWeight: 600 }}>{init.title}</span>
                <MaxTypography.Text className="cap" variant="detail" color="secondary">{new Date(init.created_at).toLocaleDateString('ru-RU', { day: 'numeric', month: 'long', year: 'numeric' })}</MaxTypography.Text>
              </Cell>
            ))}
          </UiList>
        )}
      </Main>
      <Foot>
        <Btn icon="plus" to={`${P.templates}?house=${encodeURIComponent(house.slug)}`}>
          Новая инициатива
        </Btn>
        {me.orgs?.length > 0 && <Btn kind="text" to="/uk">Кабинет УК</Btn>}
      </Foot>
    </Screen>
  );
}

// 4б. Мой дом — гость
export function HomeGuest() {
  const [searchParams] = useSearchParams();
  const meState = useApi(() => fetchMe());
  const me = meState.data;

  const linkedSlug = searchParams.get('house') ?? me?.house?.slug ?? getStartParam();
  const guestMembership = me?.memberships.find((m) => (!linkedSlug || m.house.slug === linkedSlug) && !(m.role === 'owner' && m.status === 'verified'));
  const house = guestMembership?.house;
  const premise = guestMembership?.premise;
  const houseId = house?.id;
  const canViewInitiatives = guestMembership?.status === 'verified';

  const initiativesState = useApi(() => (houseId && canViewInitiatives ? fetchInitiatives(houseId) : Promise.resolve({ initiatives: [] })), [houseId, canViewInitiatives]);

  if (meState.loading || initiativesState.loading) {
    return (
      <Screen>
        <Main>
          <Card className="card">
            <MaxTypography.Text className="t2" variant="body" color="secondary">Загрузка...</MaxTypography.Text>
          </Card>
        </Main>
      </Screen>
    );
  }

  if (meState.error || initiativesState.error || !me || !house || !premise) {
    return (
      <Screen>
        <Main>
          <Card className="card">
            <Note kind="neg">Не удалось загрузить данные.</Note>
          </Card>
        </Main>
      </Screen>
    );
  }

  const initiatives = initiativesState.data?.initiatives || [];
  const displayArea = premise.display_area_m2 ? parseM2(premise.display_area_m2) : null;
  const awaitingReview = guestMembership.status === 'pending' && guestMembership.role === 'owner';
  const rejected = guestMembership.status === 'rejected';

  return (
    <Screen>
      <HomeHeader address={house.address} isDemo={house.is_demo} />
      <Main>
        <Card className="card">
          <div className="row" style={{ gap: 12 }}>
            <Apt n={premise.number} size={56} />
            <div className="grow">
              <MaxTypography.Headline className="h3" variant="small">
                Кв. {premise.number}
                {displayArea && ` · ${fmtNum(displayArea)} м²`}
              </MaxTypography.Headline>
              <div className="row">
                <Status kind={awaitingReview ? 'paper' : rejected ? 'bad' : 'none'}>
                  {awaitingReview ? 'Ожидает проверки УК' : rejected ? 'Заявка отклонена' : guestMembership.role === 'resident' ? 'Житель' : 'Гость'}
                </Status>
              </div>
            </div>
          </div>
          <Note kind="info" icon="lock" style={{ padding: '12px 14px' }}>
            {awaitingReview
              ? 'Заявка отправлена в УК. После подтверждения вы сможете голосовать и видеть инициативы дома.'
              : rejected
                ? `УК отклонила заявку${guestMembership.rejection_reason ? `: ${guestMembership.rejection_reason}` : ''}. Для уточнения причины обратитесь в УК.`
                : 'Подтвердите, что вы собственник, — и сможете голосовать и видеть, как идут инициативы.'}
          </Note>
        </Card>

        <div className="between" style={{ padding: '6px 4px 0' }}>
          <MaxTypography.Headline className="h3" variant="small">Инициативы дома</MaxTypography.Headline>
          <MaxTypography.Text className="cap" variant="detail" color="secondary">{initiatives.length}</MaxTypography.Text>
        </div>
        <UiList>
          {initiatives.map((init) => (
            <Cell key={init.id} trail={<Status kind={getStageStatus(init.stage)}>{getStageLabel(init.stage)}</Status>}>
              <span style={{ fontWeight: 600 }}>{init.title}</span>
            </Cell>
          ))}
        </UiList>
        <MaxTypography.Text className="cap" style={{ padding: '0 4px' }} variant="detail" color="secondary">
          Подробности, цифры и голосование — после подтверждения.
        </MaxTypography.Text>
      </Main>
      <Foot>
        {awaitingReview || rejected ? <Btn to="/">К моим домам</Btn> : <Btn to="/attach">Подтвердить квартиру</Btn>}
      </Foot>
    </Screen>
  );
}
