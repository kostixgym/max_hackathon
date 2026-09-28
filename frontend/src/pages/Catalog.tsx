import { Typography as MaxTypography } from '@maxhub/max-ui';
import { Cell, Header, Main, Screen, UiList } from '../components/ui';
import { groups } from '../routes';

// Каталог всех экранов из дизайна — для разработки и показа команде.
// Когда появится настоящий сценарий входа, стартовым экраном станет «Приветствие дома».
export function Catalog() {
  return (
    <Screen>
      <Header nav={null} title="Экраны мини-приложения" />
      <Main>
        {groups.map((g) => (
          <div key={g.title} className="col catalog-group">
            <MaxTypography.Label className="sec" variant="medium-strong">{g.title}</MaxTypography.Label>
            <UiList>
              {g.screens.map((s) => (
                <Cell key={s.path} to={s.path} chevron style={{ minHeight: 52 }}>
                  <span>{s.title}</span>
                </Cell>
              ))}
            </UiList>
          </div>
        ))}
      </Main>
    </Screen>
  );
}
