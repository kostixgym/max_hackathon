import { Typography as MaxTypography } from '@maxhub/max-ui';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { Apt, Badge, Btn, Cell, DemoBadge, Foot, Kv, LinkBtn, Main, Note, Screen, Status, UiButton, type StatusKind, UiList, Card } from '../../components/ui';
import { fmtM2, fmtNum } from '../../lib/format';
import { P } from '../../paths';
import { useApi } from '../../hooks/useApi';
import { fetchMe, fetchInitiatives, getStartParam, parseM2, formatShare } from '../../lib/api';

function HomeHeader({ address, isDemo }: { address: string; isDemo: boolean }) {
  return (
    <div className="col" style={{ gap: 2, padding: '16px 20px 8px' }}>
      <div className="row">
        <MaxTypography.Headline className="h2" style={{ flex: 1 }} variant="medium">
          Мой дом
        </MaxTypography.Headline>
        {isDemo && <DemoBadge />}
      </div>
      <MaxTypography.Text className="cap" variant="detail" color="secondary">{address}</MaxTypography.Text>
    </div>
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
  const ownerMembership = me?.memberships.find((m) => m.status === 'verified' && m.role === 'owner' && (!linkedSlug || m.house.slug === linkedSlug))
    ?? me?.memberships.find((m) => m.status === 'verified' && m.role === 'owner');
  const house = ownerMembership?.house;
  const houseId = house?.id;

  const initiativesState = useApi(() => (houseId ? fetchInitiatives(houseId) : Promise.resolve({ initiatives: [] })), [houseId]);

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

  if (meState.error || initiativesState.error || !me || !ownerMembership || !house) {
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

  const owner = ownerMembership.owner;
  const premise = ownerMembership.premise;
  const initiatives = initiativesState.data?.initiatives || [];
  const activeInitiatives = initiatives.filter((i) => i.stage === 'draft' || i.stage === 'poll' || i.stage === 'demand' || i.stage === 'meeting');
  const archivedInitiatives = initiatives.filter((i) => i.stage === 'done' || i.stage === 'cancelled');
  return (
    <Screen>
      <HomeHeader address={house.address} isDemo={house.is_demo} />
      <Main>
        <Card className="card">
          <div className="row" style={{ gap: 12 }}>
            <span className="av">{me.user.first_name[0]}</span>
            <div className="grow">
              <MaxTypography.Headline className="h3" variant="small">
                {me.user.first_name} · кв. {premise.number}
              </MaxTypography.Headline>
              <div className="row">
                <Status kind="ok">Собственник</Status>
                {activeInitiatives.some((i) => i.is_initiator) && <Status kind="acc">Инициатор</Status>}
              </div>
            </div>
          </div>
          <div className="hr" />
          <Kv k="Площадь">
            <span className="row">
              <b>{premise.display_area_m2 ? fmtM2(parseM2(premise.display_area_m2) || 0) : '—'}</b>
              <Badge kind="fact" />
            </span>
          </Kv>
          {owner && (
            <>
              <Kv k="Доля">
                <span className="row">
                  <b>{formatShare(owner.share)}</b>
                  <Badge kind="fact" />
                </span>
              </Kv>
              <Kv k="Вес голоса">
                <span className="row">
                  <b style={{ fontSize: 18 }}>{fmtM2(parseM2(owner.weight_m2) || 0)}</b>
                  <Badge kind="calc" />
                </span>
              </Kv>
            </>
          )}
          <LinkBtn>Данные неверны</LinkBtn>
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
            style={{ gap: 10 }}
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
  const guestMembership = me?.memberships.find((m) => (m.role === 'guest' || m.role === 'resident') && (!linkedSlug || m.house.slug === linkedSlug))
    ?? me?.memberships.find((m) => m.role === 'guest' || m.role === 'resident');
  const house = guestMembership?.house;
  const premise = guestMembership?.premise;
  const houseId = house?.id;

  const initiativesState = useApi(() => (houseId ? fetchInitiatives(houseId) : Promise.resolve({ initiatives: [] })), [houseId]);

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
                <Status kind="none">{guestMembership?.role === 'resident' ? 'Житель' : 'Гость'}</Status>
              </div>
            </div>
          </div>
          <Note kind="info" icon="lock" style={{ padding: '12px 14px' }}>
            Подтвердите, что вы собственник, — и сможете голосовать и видеть, как идут инициативы.
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
        <Btn to={P.confirm}>Подтвердить квартиру</Btn>
      </Foot>
    </Screen>
  );
}
