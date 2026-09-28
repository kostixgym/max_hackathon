import { Typography as MaxTypography } from '@maxhub/max-ui';
import { Badge, Btn, DemoBadge, Foot, Main, Screen, StateHead, Steps, Card } from '../../components/ui';
import { approxFlats, fmtM2 } from '../../lib/format';
import { house } from '../../mocks/demo';
import { P } from '../../paths';

// 1. Приветствие дома
export function Welcome() {
  const { thresholds: t, avgAreaM2 } = house;
  const rows = [
    { pct: '10%', m2: t.demandM2, title: 'Можно обязать УК провести собрание', flats: `~${approxFlats(t.demandM2, avgAreaM2)} квартир` },
    { pct: '>50%', m2: t.quorumAboveM2, title: 'Кворум — собрание состоялось', flats: `~${approxFlats(t.quorumAboveM2 + 0.01, avgAreaM2)} квартира` },
    { pct: '2/3', m2: t.twoThirdsM2, title: 'Решение о камерах принято', flats: `~${approxFlats(t.twoThirdsM2, avgAreaM2)} квартир` },
  ];

  return (
    <Screen>
      <Main style={{ paddingTop: 20, gap: 14 }}>
        <div className="col" style={{ gap: 10, padding: '0 4px' }}>
          <div className="row">
            {house.isDemo && <DemoBadge />}
            <Badge kind="fact" />
          </div>
          <MaxTypography.Headline className="h1" variant="large-strong">{house.address}</MaxTypography.Headline>
          <MaxTypography.Text className="t2" variant="body" color="secondary">
            {house.flats} квартир · жилая площадь <b style={{ color: 'var(--text)' }}>{fmtM2(house.totalAreaM2)}</b>
          </MaxTypography.Text>
        </div>

        <Card className="card" style={{ gap: 10 }}>
          <MaxTypography.Headline className="h3" variant="small">Кто голосует и как считается голос</MaxTypography.Headline>
          <Steps
            items={[
              <>
                Голосуют только <b>подтверждённые собственники</b>.
              </>,
              <>
                Голос считается в м²: <b>площадь × доля</b>. Например, 52,3 м² × 1/2 = 26,15 м².
              </>,
              <>
                Пороги считаются от площади <b>всего дома</b>.
              </>,
            ]}
          />
        </Card>

        <Card className="card" style={{ gap: 0, padding: '4px 16px' }}>
          <div className="between" style={{ padding: '12px 0 8px' }}>
            <MaxTypography.Headline className="h3" variant="small">Три порога</MaxTypography.Headline>
            <Badge kind="calc" />
          </div>
          {rows.map((r) => (
            <div key={r.pct} className="row" style={{ gap: 14, padding: '12px 0', borderTop: '1px solid var(--line)' }}>
              <div className="col" style={{ gap: 0, width: 76, flexShrink: 0 }}>
                <b className="num" style={{ fontSize: 22, lineHeight: '28px' }}>
                  {r.pct}
                </b>
                <span className="cap num">{fmtM2(r.m2)}</span>
              </div>
              <div className="grow">
                <span style={{ fontWeight: 600 }}>{r.title}</span>
                <MaxTypography.Text className="cap" variant="detail" color="secondary">{r.flats}</MaxTypography.Text>
              </div>
            </div>
          ))}
        </Card>
      </Main>
      <Foot>
        <Btn to={P.apartment}>Выбрать квартиру</Btn>
      </Foot>
    </Screen>
  );
}

// 1б. Нет ссылки на дом
export function NoLink() {
  return (
    <Screen>
      <Main style={{ justifyContent: 'center', alignItems: 'stretch', gap: 16, padding: 24 }}>
        <StateHead
          icon="qr"
          tone="acc"
          title="Откройте приложение по ссылке или QR-коду вашего дома"
          text="Так мы поймём, о каком доме речь. Ссылку обычно присылают в чат дома, QR-код висит на доске объявлений в подъезде."
        />
      </Main>
      <Foot>
        <Btn icon="scan">Сканировать QR-код</Btn>
        <Btn kind="text" to={P.welcome}>
          Посмотреть демо-дом
        </Btn>
      </Foot>
    </Screen>
  );
}
