import type { ComponentType } from 'react';
import { P } from './paths';
import { Welcome, NoLink } from './pages/onboarding/Welcome';
import { Apartment } from './pages/onboarding/Apartment';
import { Confirm, OwnerList, Pending, Rejected, Taken } from './pages/onboarding/Confirm';
import { Home, HomeGuest } from './pages/home/Home';
import { Templates, TemplateForm } from './pages/initiative/Templates';
import { InitPoll, InitDemand, InitResident } from './pages/initiative/Initiative';
import { Vote, Survey, Counted, PollProgress } from './pages/poll/Poll';
import { PathChoice, Demand, Countdown, Overdue } from './pages/demand/Demand';
import { UkHouses, UkDemands, UkCreateMeeting, UkRequests } from './pages/uk/Uk';
import { Loading, EmptyState, ErrorState, AuthState } from './pages/states/States';
import { MyHouses, MyHousesEmpty } from './pages/legacy/MyHouses';
import { AttachHouse, Entry } from './pages/Entry';
import { InitiativeView, PollProgressPage } from './pages/initiative/InitiativeView';
import { VotePage, SurveyPage, CountedPage } from './pages/poll/VoteFlow';
import { DemandCreatePage, DemandPage, PathChoicePage } from './pages/demand/DemandFlow';
import { MeetingCreatePage } from './pages/meeting/MeetingCreate';
import { Meeting, Tracker, WalkList, Receive, EnterDecisions, ResultPreview, Result } from './pages/meeting/Meeting';
import { UIKit } from './pages/UIKit';

export type ScreenRoute = { path: string; title: string; Component: ComponentType };
export type ScreenGroup = { title: string; screens: ScreenRoute[] };

export const applicationRoutes = [
  { path: P.uiKit, Component: UIKit },
  // Keep the earlier transposed spelling working for links already shared.
  { path: '/iu/kit', Component: UIKit },
  { path: '/', Component: Entry },
  { path: '/attach', Component: AttachHouse },
  { path: '/home', Component: Home },
  { path: '/home/guest', Component: HomeGuest },
  { path: '/templates', Component: Templates },
  { path: '/templates/:code', Component: TemplateForm },
  { path: '/initiatives/:id', Component: InitiativeView },
  { path: '/initiatives/:id/progress', Component: PollProgressPage },
  { path: '/initiatives/:id/vote', Component: VotePage },
  { path: '/initiatives/:id/survey', Component: SurveyPage },
  { path: '/initiatives/:id/counted', Component: CountedPage },
  { path: '/initiatives/:id/path', Component: PathChoicePage },
  { path: '/initiatives/:id/demand/new', Component: DemandCreatePage },
  { path: '/demands/:id', Component: DemandPage },
  { path: '/initiatives/:id/meeting/new', Component: MeetingCreatePage },
  { path: '/meetings/:id', Component: Meeting },
  { path: '/meetings/:id/tracker', Component: Tracker },
  { path: '/meetings/:id/walk', Component: WalkList },
  { path: '/meetings/:id/receive', Component: Receive },
  { path: '/meetings/:id/enter/:ballotId', Component: EnterDecisions },
  { path: '/meetings/:id/result-preview', Component: ResultPreview },
  { path: '/meetings/:id/result', Component: Result },
  { path: '/uk', Component: UkHouses },
  { path: '/uk/demands', Component: UkDemands },
  { path: '/uk/meeting/new', Component: UkCreateMeeting },
  { path: '/uk/requests', Component: UkRequests },
];

// Экраны в порядке артбордов дизайна «ОСС в MAX — мини-приложение».
export const groups: ScreenGroup[] = [
  {
    title: 'Вход: дом, квартира, подтверждение',
    screens: [
      { path: P.welcome, title: '1. Приветствие дома', Component: Welcome },
      { path: P.noLink, title: '1б. Нет ссылки на дом', Component: NoLink },
      { path: P.apartment, title: '2. Выбор квартиры', Component: Apartment },
      { path: P.confirm, title: '3. Подтверждение собственника', Component: Confirm },
      { path: P.ownerList, title: '3б. Выбрать себя в списке', Component: OwnerList },
      { path: P.pending, title: '3в. Ожидает УК', Component: Pending },
      { path: P.rejected, title: '3г. Отклонено с причиной', Component: Rejected },
      { path: P.taken, title: '3д. Уже подтверждён за другим', Component: Taken },
    ],
  },
  {
    title: 'Мой дом и новая инициатива',
    screens: [
      { path: P.home, title: '4. Мой дом — собственник', Component: Home },
      { path: P.homeGuest, title: '4б. Мой дом — гость', Component: HomeGuest },
      { path: P.templates, title: '5. Выбор шаблона', Component: Templates },
      { path: P.templateForm, title: '6. Форма «Видеонаблюдение»', Component: TemplateForm },
      { path: P.initPoll, title: '7а. Инициатива · Опрос', Component: InitPoll },
      { path: P.initDemand, title: '7б. Инициатива · Требование', Component: InitDemand },
      { path: P.initResident, title: '7в. Инициатива · жилец', Component: InitResident },
    ],
  },
  {
    title: 'Опрос и требование в УК',
    screens: [
      { path: P.vote, title: '8. Голос в опросе', Component: Vote },
      { path: P.survey, title: '8б. Анкета после «За»', Component: Survey },
      { path: P.counted, title: '8в. Голос учтён', Component: Counted },
      { path: P.pollProgress, title: '9. Ход опроса — инициатор', Component: PollProgress },
      { path: P.path, title: '10. Выбор пути', Component: PathChoice },
      { path: P.demand, title: '11. Требование в УК', Component: Demand },
      { path: P.countdown, title: '11б. Отсчёт 45 дней', Component: Countdown },
      { path: P.overdue, title: '11в. Просрочка', Component: Overdue },
    ],
  },
  {
    title: 'Собрание, подсчёт и итог',
    screens: [
      { path: P.meeting, title: '12. Карточка собрания', Component: Meeting },
      { path: P.tracker, title: '13. Трекер квартир', Component: Tracker },
      { path: P.walkList, title: '13б. Список обхода', Component: WalkList },
      { path: P.receive, title: '14. Приём бюллетеня', Component: Receive },
      { path: P.enter, title: '15. Внесение решений', Component: EnterDecisions },
      { path: P.preview, title: '16. Проверка и фиксация итога', Component: ResultPreview },
      { path: P.result, title: '17. Итог', Component: Result },
    ],
  },
  {
    title: 'Кабинет УК',
    screens: [
      { path: P.ukHouses, title: '18. Дома УК', Component: UkHouses },
      { path: P.ukDemands, title: '19. Входящие требования', Component: UkDemands },
      { path: P.ukCreateMeeting, title: '20. Создание собрания', Component: UkCreateMeeting },
      { path: P.ukRequests, title: '21. Заявки на подтверждение', Component: UkRequests },
    ],
  },
  {
    title: 'Состояния',
    screens: [
      { path: P.loading, title: 'Загрузка', Component: Loading },
      { path: P.empty, title: 'Пусто', Component: EmptyState },
      { path: P.error, title: 'Ошибка', Component: () => <ErrorState /> },
      { path: P.auth, title: 'Ошибка авторизации', Component: () => <AuthState /> },
    ],
  },
  {
    title: 'v1 · Мои дома',
    screens: [
      { path: P.v1Houses, title: 'Главная — 5 домов', Component: MyHouses },
      { path: P.v1Empty, title: 'Главная — 0 домов', Component: MyHousesEmpty },
    ],
  },
];
