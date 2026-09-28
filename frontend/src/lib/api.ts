// API client for backend endpoints. Contracts match backend's httpapi package.

const API_BASE = '/api/v1';

declare global {
  interface Window {
    WebApp?: {
      initData?: string;
      requestContact?: () => Promise<{ phone: string; authDate: string; hash: string }>;
      initDataUnsafe?: {
        start_param?: string;
        user?: { first_name?: string; last_name?: string; username?: string; photo_url?: string };
      };
    };
  }
}

function authHeaders(): Record<string, string> {
  const initData = window.WebApp?.initData;
  if (initData) return { 'X-Max-Init-Data': initData };
  const devUserId = import.meta.env.VITE_DEV_USER_ID;
  return devUserId ? {
    'X-Dev-User-Id': devUserId,
    ...(import.meta.env.VITE_DEV_START_PARAM ? { 'X-Dev-Start-Param': import.meta.env.VITE_DEV_START_PARAM } : {}),
  } : {};
}

export function getStartParam(): string | null {
  const maxParam = window.WebApp?.initDataUnsafe?.start_param;
  if (maxParam) return maxParam;
  return import.meta.env.VITE_DEV_START_PARAM || null;
}

// Error response from backend
interface ApiError {
  code: string;
  message: string;
}

class ApiException extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
    this.name = 'ApiException';
  }
}

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${url}`, {
    ...options,
    headers: {
      ...authHeaders(),
      'Content-Type': 'application/json',
      ...options?.headers,
    },
  });

  if (!res.ok) {
    const body = await res.json().catch(() => null);
    const error: ApiError = body?.error ?? body ?? { code: 'unknown', message: 'Неизвестная ошибка' };
    throw new ApiException(res.status, error.code || 'unknown', error.message || 'Неизвестная ошибка');
  }

  return res.json();
}

export async function downloadPDF(url: string, filename: string): Promise<void> {
  const res = await fetch(`${API_BASE}${url}`, { headers: authHeaders() });
  if (!res.ok) {
    const body = await res.json().catch(() => null);
    throw new ApiException(res.status, body?.error?.code ?? 'unknown', body?.error?.message ?? 'Не удалось скачать документ');
  }
  const objectURL = URL.createObjectURL(await res.blob());
  const link = document.createElement('a');
  link.href = objectURL;
  link.download = filename;
  document.body.append(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(objectURL);
}

// Types matching backend JSON responses

export interface MeUser {
  id: string;
  first_name: string;
}

export function getDisplayUser(user?: MeUser) {
  const maxUser = window.WebApp?.initDataUnsafe?.user;
  const hasMaxSession = Boolean(window.WebApp?.initData);
  return {
    firstName: maxUser?.first_name || (hasMaxSession ? user?.first_name : import.meta.env.VITE_DEV_USER_FIRST_NAME) || 'Разработчик',
    lastName: maxUser?.last_name || (hasMaxSession ? '' : import.meta.env.VITE_DEV_USER_LAST_NAME) || '',
    username: maxUser?.username || (hasMaxSession ? '' : import.meta.env.VITE_DEV_USER_USERNAME) || '',
    photoUrl: maxUser?.photo_url || '',
  };
}

export interface HouseThresholds {
  demand_m2: string; // decimal string like "300.00"
  quorum_above_m2: string;
  two_thirds_m2: string;
}

export interface HouseJSON {
  id: string;
  slug: string;
  address: string;
  region: string;
  locality?: string;
  street?: string;
  house_number?: string;
  building?: string;
  structure?: string;
  fias_id?: string;
  is_demo: boolean;
  premises_count: number;
  registry_version: number | null;
  total_area_m2: string | null; // decimal string like "3000.00"
  thresholds: HouseThresholds | null;
}

export interface MembershipHouse {
  id: string;
  slug: string;
  address: string;
  region: string;
  is_demo: boolean;
}

export interface MembershipPremise {
  id: string;
  number: string;
  kind: string; // "residential" | "nonresidential" | "parking"
  entrance: number | null;
  floor: number | null;
  display_area_m2: string | null;
}

export interface OwnerShare {
  numerator: number;
  denominator: number;
}

export interface Owner {
  id: string;
  masked_name: string;
  kind: string; // "person" | "organization" | "municipality"
  share: OwnerShare;
  weight_m2: string; // decimal string
  premise?: {
    id: string;
    number: string;
  };
}

export interface Membership {
  id: string;
  role: string; // "guest" | "resident" | "owner"
  status: string; // "pending" | "verified" | "rejected" | "revoked"
  method: string | null; // "phone" | "account" | "uk_manual" | "demo"
  rejection_reason?: string | null;
  house: MembershipHouse;
  premise: MembershipPremise;
  owner: Owner | null;
}

export interface MeResponse {
  user: MeUser;
  dev_mode: boolean;
  house: HouseJSON | null; // present if opened via house invite link
  memberships: Membership[];
  orgs: Org[];
}

export interface Org {
  id: string;
  name: string;
  type: string;
  role?: string;
}

export interface OrgHouse {
  id: string;
  slug: string;
  address: string;
  region: string;
  is_demo: boolean;
}

export async function fetchOrgs(): Promise<{ orgs: Org[] }> {
  return request('/orgs');
}

export async function fetchOrgHouses(orgId: string): Promise<{ houses: OrgHouse[] }> {
  return request(`/orgs/${encodeURIComponent(orgId)}/houses`);
}

export async function becomeDemoStaff(houseSlug: string): Promise<{ org: Org; role: string }> {
  return request(`/houses/${encodeURIComponent(houseSlug)}/demo-staff`, { method: 'POST', body: '{}' });
}

export interface OwnersResponse {
  owners: Owner[];
}

// API functions

/**
 * GET /api/v1/me
 * Returns current user profile, house from startParam if present, and all memberships.
 */
export async function fetchMe(): Promise<MeResponse> {
  return request<MeResponse>('/me');
}

/**
 * GET /api/v1/houses/:house
 * Returns house details with thresholds calculated from total area.
 */
export async function fetchHouse(houseSlug: string): Promise<HouseJSON> {
  return request<HouseJSON>(`/houses/${encodeURIComponent(houseSlug)}`);
}

export async function searchHouses(query: string): Promise<{ houses: HouseJSON[] }> {
  return request(`/houses?q=${encodeURIComponent(query)}`);
}

export async function attachGuest(houseSlug: string, premiseNumber: string): Promise<{ membership_id: string; role: string; status: string; next_step: string }> {
  return request(`/houses/${encodeURIComponent(houseSlug)}/memberships`, {
    method: 'POST', body: JSON.stringify({ premise_number: premiseNumber }),
  });
}

export async function verifyMembershipPhone(membershipId: string, contact: { phone: string; auth_date: string; hash: string }): Promise<{ role: string; status: string; method: string }> {
  return request(`/memberships/${encodeURIComponent(membershipId)}/verify/phone`, {
    method: 'POST', body: JSON.stringify(contact),
  });
}

export async function fetchOwnerClaimCandidates(membershipId: string): Promise<{ owners: Owner[] }> {
  return request(`/memberships/${encodeURIComponent(membershipId)}/claim-candidates`);
}

export async function submitOwnerClaim(membershipId: string, ownerId: string): Promise<void> {
  await request(`/memberships/${encodeURIComponent(membershipId)}/claim`, { method: 'POST', body: JSON.stringify({ owner_id: ownerId }) });
}

export interface OwnerRequest { membership_id: string; owner: Owner; }
export async function fetchOrgOwnerRequests(orgId: string): Promise<{ requests: OwnerRequest[] }> {
  return request(`/orgs/${encodeURIComponent(orgId)}/owner-requests`);
}
export async function decideOrgOwnerRequest(orgId: string, membershipId: string, approve: boolean): Promise<void> {
  await request(`/orgs/${encodeURIComponent(orgId)}/owner-requests/${encodeURIComponent(membershipId)}/${approve ? 'approve' : 'reject'}`, {
    method: 'POST', body: approve ? '{}' : JSON.stringify({ reason: 'Данные заявки не совпали с реестром собственников' }),
  });
}

/**
 * GET /api/v1/premises/:premiseID/owners
 * Returns list of owners for a premise (requires verified membership to access).
 */
export async function fetchPremiseOwners(premiseId: string): Promise<OwnersResponse> {
  return request<OwnersResponse>(`/premises/${encodeURIComponent(premiseId)}/owners`);
}

/**
 * GET /api/v1/houses/:house/meeting-officer-candidates
 * Returns verified owners eligible to be meeting officers (chairman/secretary).
 */
export async function fetchMeetingOfficerCandidates(houseSlug: string): Promise<OwnersResponse> {
  return request<OwnersResponse>(`/houses/${encodeURIComponent(houseSlug)}/meeting-officer-candidates`);
}

/**
 * GET /api/v1/healthz
 */
export async function healthCheck(): Promise<{ status: string }> {
  return request('/healthz');
}

/**
 * GET /api/v1/readyz
 */
export async function readinessCheck(): Promise<{ status: string }> {
  return request('/readyz');
}

// Utility: parse decimal string from API to number for display
export function parseM2(str: string | null): number | null {
  if (!str) return null;
  const num = parseFloat(str);
  return isNaN(num) ? null : num;
}

// Utility: format share as "1/2" or "1" if denominator is 1
export function formatShare(share: OwnerShare): string {
  return share.denominator === 1 ? String(share.numerator) : `${share.numerator}/${share.denominator}`;
}

// Templates

export interface TemplateSummary {
  code: string;
  name: string;
  version: number;
  description: string;
}

export interface AgendaItem {
  position: number;
  text: string;
  majority_rule: string;
  legal_reference?: string;
}

export interface TemplateDetail {
  code: string;
  name: string;
  version: number;
  description: string;
  params_schema: any; // JSON Schema
  ui_schema: any; // UI Schema for form rendering
  agenda_items: AgendaItem[];
}

export interface TemplatesResponse {
  templates: TemplateSummary[];
}

export async function fetchTemplates(): Promise<TemplatesResponse> {
  return request<TemplatesResponse>('/templates');
}

export async function fetchTemplate(code: string): Promise<TemplateDetail> {
  return request<TemplateDetail>(`/templates/${encodeURIComponent(code)}`);
}

// Initiatives

export interface InitiativeSummary {
  id: string;
  title: string;
  stage: string; // "draft" | "poll" | "demand" | "meeting" | "done" | "cancelled"
  path: string | null; // "uk_demand" | "initiative_meeting" | null
  poll_ends_at: string | null; // RFC3339
  created_at: string; // RFC3339
  is_initiator: boolean;
}

export interface InitiativesResponse {
  initiatives: InitiativeSummary[];
}

export interface TemplateRef {
  code: string;
  name: string;
  version: number;
}

export interface InitiativeAction {
  code: string; // "start_poll" | "create_demand" | "create_meeting" | etc
  allowed: boolean;
  reason_code?: string;
}

export interface MyVoteCard {
  choice: string; // "for" | "against"
  weight_m2: string;
  official_channel?: string; // "gosuslugi" | "paper"
  willing_to_help: boolean;
  updated_at: string; // RFC3339
}

export interface InitiativeCard {
  id: string;
  house_id: string;
  title: string;
  description: string;
  stage: string;
  path: string | null;
  poll_ends_at: string | null;
  created_at: string;
  is_initiator: boolean;
  template: TemplateRef | null;
  params: any; // template params
  registry_version: number;
  total_area_m2: string;
  thresholds: HouseThresholds;
  agenda_items: AgendaItem[];
  my_vote: MyVoteCard | null;
  allowed_actions: InitiativeAction[];
}

export interface CreateInitiativeInput {
  template_code: string;
  title: string;
  description: string;
  params?: any;
}

export interface InitiativeResponse {
  id: string;
  house_id: string;
  title: string;
  description: string;
  stage: string;
  poll_ends_at: string | null;
  is_initiator: boolean;
  registry_version?: number;
  agenda_items?: AgendaItem[];
}

export async function fetchInitiatives(houseId: string): Promise<InitiativesResponse> {
  return request<InitiativesResponse>(`/houses/${encodeURIComponent(houseId)}/initiatives`);
}

export async function fetchInitiative(id: string): Promise<InitiativeCard> {
  return request<InitiativeCard>(`/initiatives/${encodeURIComponent(id)}`);
}

export async function createInitiative(houseId: string, input: CreateInitiativeInput): Promise<InitiativeResponse> {
  return request<InitiativeResponse>(`/houses/${encodeURIComponent(houseId)}/initiatives`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function startPoll(initiativeId: string, endsAt?: string): Promise<InitiativeResponse> {
  return request<InitiativeResponse>(`/initiatives/${encodeURIComponent(initiativeId)}/start-poll`, {
    method: 'POST',
    body: JSON.stringify(endsAt ? { ends_at: endsAt } : {}),
  });
}

export async function selectPathB(initiativeId: string): Promise<void> {
  await request(`/initiatives/${encodeURIComponent(initiativeId)}/select-path-b`, { method: 'POST', body: '{}' });
}

// Poll & Voting

export interface PollProgress {
  for_m2: string;
  against_m2: string;
  total_m2: string;
  demand_m2: string;
  demand_reached: boolean;
  poll_ends_at: string | null;
  initiative_id: string;
  title: string;
  stage: string;
  for_percent: string;
  votes_for: number;
  votes_against: number;
  thresholds: HouseThresholds;
}

export interface MyVoteResponse {
  choice: string;
  weight_m2: string;
  premises: string;
  updated_at: string;
}

export interface VoteInput {
  choice: 'for' | 'against';
  official_channel?: 'gosuslugi' | 'paper';
  willing_to_help?: boolean;
}

export async function fetchPollProgress(initiativeId: string): Promise<PollProgress> {
  return request<PollProgress>(`/initiatives/${encodeURIComponent(initiativeId)}/poll`);
}

export async function castVote(initiativeId: string, vote: VoteInput): Promise<MyVoteResponse> {
  return request<MyVoteResponse>(`/initiatives/${encodeURIComponent(initiativeId)}/my-vote`, {
    method: 'PUT',
    body: JSON.stringify(vote),
  });
}

// Demand

export interface DemandRecord {
  id: string;
  initiative_id: string;
  channel: 'paper' | 'gosuslugi_dom';
  status: 'draft' | 'delivered';
  support_m2: string;
  created_at: string;
  delivered_at: string | null;
  uk_due_at: string | null;
  overdue: boolean;
}

export interface OrgDemand extends DemandRecord {
  house_id: string;
  house_address: string;
  initiative_title: string;
  house: { id: string; address: string };
}

export async function fetchOrgDemands(orgId: string): Promise<{ demands: OrgDemand[] }> {
  return request(`/orgs/${encodeURIComponent(orgId)}/demands`);
}

export async function createDemand(initiativeId: string, channel: DemandRecord['channel']): Promise<DemandRecord> {
  return request(`/initiatives/${encodeURIComponent(initiativeId)}/demand`, {
    method: 'POST', body: JSON.stringify({ channel }),
  });
}

export async function fetchDemand(id: string): Promise<DemandRecord> {
  return request(`/demands/${encodeURIComponent(id)}`);
}

export async function fetchInitiativeDemand(initiativeId: string): Promise<DemandRecord> {
  return request(`/initiatives/${encodeURIComponent(initiativeId)}/demand`);
}

export async function markDemandDelivered(id: string, deliveredAt: string): Promise<DemandRecord> {
  return request(`/demands/${encodeURIComponent(id)}/mark-delivered`, {
    method: 'POST', body: JSON.stringify({ delivered_at: deliveredAt }),
  });
}

export async function downloadDemandPDF(id: string): Promise<void> {
  const response = await fetch(`${API_BASE}/demands/${encodeURIComponent(id)}/pdf`, { headers: authHeaders() });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(body?.error?.message ?? 'Не удалось скачать требование');
  }
  const url = URL.createObjectURL(await response.blob());
  const link = document.createElement('a');
  link.href = url;
  link.download = 'trebovanie-v-uk.pdf';
  link.click();
  window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
}

// Meetings

export interface MeetingHouse {
  id: string;
  address: string;
}

export interface Officer {
  owner_id: string;
  masked_name: string;
}

export interface MeetingAgendaItem {
  id: string;
  position: number;
  text: string;
  majority_rule: string;
}

export interface MeetingProgress {
  ballots_total: number;
  ballots_received: number;
  participants_m2: string;
  total_m2: string;
  quorum_above_m2: string;
}

export interface Meeting {
  id: string;
  initiative_id: string;
  title: string;
  house: MeetingHouse;
  attempt: number;
  form: string; // "gis_electronic" | "paper_absentee"
  status: string; // "notice" | "voting" | "counting" | "finalized"
  notice_at: string; // RFC3339
  voting_starts_at: string;
  voting_ends_at: string;
  chair: Officer;
  secretary: Officer;
  agenda_items: MeetingAgendaItem[];
  progress: MeetingProgress;
  is_admin: boolean;
  outcome: string | null; // "accepted" | "rejected" | null
  finalized_at: string | null;
}

export interface TrackerBallot {
  id: string;
  premise_number: string;
  entrance: number | null;
  owner_masked_name: string;
  weight_m2: string;
  status: string; // "pending" | "received" | "counted"
}

export interface TrackerSummary {
  ballots_total: number;
  received: number;
  counted: number;
  participants_m2: string;
}

export interface MeetingTracker {
  summary: TrackerSummary;
  ballots: TrackerBallot[];
}

export interface CreateMeetingInput {
  path?: 'A' | 'B';
  form: 'gis_electronic' | 'paper_absentee';
  notice_at: string; // RFC3339
  voting_starts_at: string;
  voting_ends_at: string;
  chair_owner_id: string;
  secretary_owner_id: string;
}

export interface Decision {
  agenda_item_id: string;
  choice: 'for' | 'against' | 'abstain';
}

export interface BallotDecisionsInput {
  decisions: Decision[];
}

export interface ReceivedBallot {
  ballot_id: string;
  status: string;
  received_at: string;
}

export interface BallotDecisions {
  ballot_id: string;
  status: string;
  decisions: Decision[];
}

export interface AgendaResult {
  agenda_item_id: string;
  position: number;
  text: string;
  majority_rule: string;
  for_m2: string;
  against_m2: string;
  abstain_m2: string;
  accepted: boolean;
}

export interface MeetingResult {
  participants_m2: string;
  total_m2: string;
  quorum_reached: boolean;
  agenda_results: AgendaResult[];
}

export interface MeetingFinal {
  meeting_id: string;
  outcome: string;
  finalized_at: string;
  participants_m2: string;
  total_m2: string;
  quorum_reached: boolean;
  results: AgendaResult[];
}

export async function createMeeting(initiativeId: string, input: CreateMeetingInput): Promise<Meeting> {
  return request<Meeting>(`/initiatives/${encodeURIComponent(initiativeId)}/meetings`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function fetchMeeting(meetingId: string): Promise<Meeting> {
  return request<Meeting>(`/meetings/${encodeURIComponent(meetingId)}`);
}

export async function fetchInitiativeMeeting(initiativeId: string): Promise<Meeting> {
  return request<Meeting>(`/initiatives/${encodeURIComponent(initiativeId)}/meeting`);
}

export async function fetchMeetingTracker(meetingId: string): Promise<MeetingTracker> {
  return request<MeetingTracker>(`/meetings/${encodeURIComponent(meetingId)}/tracker`);
}

export async function receiveBallot(meetingId: string, ballotId: string): Promise<ReceivedBallot> {
  return request<ReceivedBallot>(`/meetings/${encodeURIComponent(meetingId)}/ballots/receive`, {
    method: 'POST',
    body: JSON.stringify({ ballot_id: ballotId }),
  });
}

export async function recordBallotDecisions(ballotId: string, input: BallotDecisionsInput): Promise<BallotDecisions> {
  return request<BallotDecisions>(`/ballots/${encodeURIComponent(ballotId)}/decisions`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
}

export async function fetchMeetingResultPreview(meetingId: string): Promise<MeetingResult> {
  return request<MeetingResult>(`/meetings/${encodeURIComponent(meetingId)}/result-preview`);
}

export async function finalizeMeeting(meetingId: string): Promise<MeetingFinal> {
  return request<MeetingFinal>(`/meetings/${encodeURIComponent(meetingId)}/finalize`, {
    method: 'POST',
  });
}

// Demo shortcuts

export interface DemoMembershipInput {
  premise_number: string;
  owner_index?: number;
}

export interface DemoMembershipResponse {
  membership_id: string;
  premise: string;
  role: string;
  status: string;
  method: string;
}

export async function confirmDemoMembership(houseSlug: string, input: DemoMembershipInput): Promise<DemoMembershipResponse> {
  return request<DemoMembershipResponse>(`/houses/${encodeURIComponent(houseSlug)}/demo-membership`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function demoFinishVoting(meetingId: string): Promise<Meeting> {
  return request<Meeting>(`/meetings/${encodeURIComponent(meetingId)}/demo/finish-voting`, {
    method: 'POST',
  });
}

export async function demoFillBallots(meetingId: string): Promise<Meeting> {
  return request<Meeting>(`/meetings/${encodeURIComponent(meetingId)}/demo/fill-ballots`, {
    method: 'POST',
  });
}
