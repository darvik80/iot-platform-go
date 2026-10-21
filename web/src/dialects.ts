export type TopicStyle = 'short' | 'full' | 'alink' | 'thingsboard';

export type CredentialKind = 'native' | 'secret' | 'token';
// native — username = product.device, password = access token (short, full)
// secret — deviceSecret, HMAC считается прошивкой (alink)
// token  — username = access token, пароля нет (thingsboard)

export interface DialectInfo {
  credential: CredentialKind;
  shortTopics: boolean;
  labelKey: string;
  topicsKey: string;
  hintKey?: string;
}

const DIALECTS: Record<TopicStyle, DialectInfo> = {
  short: {
    credential: 'native',
    shortTopics: true,
    labelKey: 'devices.style.short',
    topicsKey: 'devices.topics.short',
  },
  full: {
    credential: 'native',
    shortTopics: false,
    labelKey: 'devices.style.full',
    topicsKey: 'devices.topics.full',
  },
  alink: {
    credential: 'secret',
    shortTopics: true,
    labelKey: 'devices.style.alink',
    topicsKey: 'devices.topics.alink',
    hintKey: 'devices.alinkHint',
  },
  thingsboard: {
    credential: 'token',
    shortTopics: true,
    labelKey: 'devices.style.thingsboard',
    topicsKey: 'devices.topics.thingsboard',
    hintKey: 'devices.thingsboardHint',
  },
};

const AUTO: DialectInfo = {
  credential: 'native',
  shortTopics: true,
  labelKey: 'devices.styleProductDefault',
  topicsKey: 'devices.topics.auto',
};

export const DIALECT_IDS = Object.keys(DIALECTS) as TopicStyle[];

export function dialectOf(style?: string | null): DialectInfo {
  const d = style ? (DIALECTS as Record<string, DialectInfo | undefined>)[style] : undefined;
  return d ?? AUTO;
}
