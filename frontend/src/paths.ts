// Адреса экранов. Один источник для роутера, ссылок между экранами и каталога.
export const P = {
  catalog: '/__screens',
  uiKit: '/ui/kit',

  welcome: '/welcome',
  noLink: '/no-link',
  apartment: '/apartment',
  confirm: '/confirm',
  ownerList: '/confirm/owners',
  pending: '/confirm/pending',
  rejected: '/confirm/rejected',
  taken: '/confirm/taken',

  home: '/home',
  homeGuest: '/home/guest',

  templates: '/templates',
  templateForm: '/templates/video_surveillance',
  initPoll: '/initiative/poll',
  initDemand: '/initiative/demand',
  initResident: '/initiative/resident',

  vote: '/poll/vote',
  survey: '/poll/survey',
  counted: '/poll/counted',
  pollProgress: '/poll/progress',

  path: '/path',
  demand: '/demand',
  countdown: '/demand/countdown',
  overdue: '/demand/overdue',

  meeting: '/meeting',
  tracker: '/meeting/tracker',
  walkList: '/meeting/walk',
  receive: '/meeting/receive',
  enter: '/meeting/enter',
  preview: '/meeting/preview',
  result: '/meeting/result',

  ukHouses: '/uk',
  ukDemands: '/uk/demands',
  ukCreateMeeting: '/uk/meeting/new',
  ukRequests: '/uk/requests',

  loading: '/state/loading',
  empty: '/state/empty',
  error: '/state/error',
  auth: '/state/auth',

  v1Houses: '/v1/houses',
  v1Empty: '/v1/houses-empty',
} as const;
