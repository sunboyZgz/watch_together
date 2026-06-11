import http from 'k6/http';
import ws from 'k6/ws';
import { check, fail, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

const vuCount = Number(__ENV.VUS || '20');
const testDuration = __ENV.DURATION || '60s';
const baseURL = (__ENV.BASE_URL || 'http://127.0.0.1:8080').replace(/\/$/, '');
const wsURL = (__ENV.WS_URL || baseURL.replace(/^http/, 'ws') + '/ws').replace(/\/$/, '');
const mediaID = __ENV.MEDIA_ID || '';
const roomCodeFromEnv = __ENV.ROOM_CODE || '';
const autoRegister = (__ENV.AUTO_REGISTER || 'true').toLowerCase() !== 'false';
const accountPrefix = __ENV.ACCOUNT_PREFIX || `load_${Date.now()}`;
const password = __ENV.PASSWORD || 'LoadTest123!';
const controlIntervalMs = Number(__ENV.CONTROL_INTERVAL_MS || '1500');
const controlPattern = (__ENV.CONTROL_PATTERN || 'mixed').toLowerCase();
const holdOpenMs = Number(__ENV.HOLD_OPEN_MS || '60000');

export const options = {
  vus: vuCount,
  duration: testDuration,
  thresholds: {
    ws_connect_errors: ['count==0'],
    ws_protocol_errors: ['rate<0.01'],
    room_state_received: [`count>=${vuCount}`],
    join_latency_ms: ['p(95)<2000'],
  },
};

const wsConnectErrors = new Counter('ws_connect_errors');
const wsProtocolErrors = new Rate('ws_protocol_errors');
const wsMessages = new Counter('ws_messages');
const roomStateReceived = new Counter('room_state_received');
const controlBroadcasts = new Counter('control_broadcasts');
const heartbeatReceived = new Counter('heartbeat_received');
const joinLatencyMs = new Trend('join_latency_ms');

export function setup() {
  const users = loadUsers();
  if (users.length < vuCount) {
    fail(`Need at least ${vuCount} users, got ${users.length}`);
  }

  let roomCode = roomCodeFromEnv;
  if (!roomCode) {
    if (!mediaID) {
      fail('MEDIA_ID is required when ROOM_CODE is not provided');
    }
    roomCode = createRoom(users[0].token, mediaID);
  }

  for (let index = 1; index < users.length; index += 1) {
    joinRoom(users[index].token, roomCode);
  }

  return {
    roomCode,
    users: users.slice(0, vuCount),
  };
}

export default function (data) {
  const user = data.users[__VU - 1];
  const isHost = __VU === 1;
  let seq = 0;
  let joined = false;
  let controlIndex = 0;
  const startedAt = Date.now();

  const response = ws.connect(
    wsURL,
    {
      headers: {
        Authorization: `Bearer ${user.token}`,
      },
    },
    (socket) => {
      socket.on('open', () => {
        socket.send(JSON.stringify({
          type: 'join_room',
          payload: {
            roomId: data.roomCode,
            userId: user.userId,
            deviceId: user.deviceId || `load-device-${__VU}`,
          },
        }));
      });

      socket.on('message', (raw) => {
        wsMessages.add(1);
        let envelope;
        try {
          envelope = JSON.parse(raw);
        } catch (error) {
          wsProtocolErrors.add(1);
          return;
        }

        const payload = envelope.payload || {};
        if (envelope.type === 'room_state') {
          if (!joined) {
            joined = true;
            joinLatencyMs.add(Date.now() - startedAt);
          }
          seq = payload.seq || seq;
          roomStateReceived.add(1);
          return;
        }

        if (envelope.type === 'play' ||
          envelope.type === 'pause' ||
          envelope.type === 'seek' ||
          envelope.type === 'set_playback_rate' ||
          envelope.type === 'ended') {
          seq = payload.seq || seq;
          controlBroadcasts.add(1);
          return;
        }

        if (envelope.type === 'heartbeat') {
          heartbeatReceived.add(1);
          socket.send(JSON.stringify({
            type: 'heartbeat_ack',
            payload: {
              serverTimeMs: payload.serverTimeMs,
              clientTimeMs: Date.now(),
            },
          }));
          return;
        }

        if (envelope.type === 'error') {
          wsProtocolErrors.add(1);
        }
      });

      socket.on('error', () => {
        wsProtocolErrors.add(1);
      });

      if (isHost) {
        socket.setInterval(() => {
          if (!joined || seq <= 0) return;
          const control = nextControl(data.roomCode, user.userId, seq, controlIndex);
          controlIndex += 1;
          socket.send(JSON.stringify(control));
        }, controlIntervalMs);
      }

      socket.setTimeout(() => {
        socket.close();
      }, holdOpenMs);
    },
  );

  check(response, {
    'ws connected': (res) => res && res.status === 101,
  }) || wsConnectErrors.add(1);

  sleep(1);
}

function loadUsers() {
  if (__ENV.USER_TOKENS) {
    return JSON.parse(__ENV.USER_TOKENS);
  }
  if (!autoRegister) {
    fail('USER_TOKENS is required when AUTO_REGISTER=false');
  }

  const users = [];
  for (let index = 0; index < vuCount; index += 1) {
    const account = `${accountPrefix}_${index}`;
    const nickname = index === 0 ? 'load_host' : `load_viewer_${index}`;
    users.push(registerOrLogin(account, password, nickname));
  }
  return users;
}

function registerOrLogin(account, password, nickname) {
  const registerResponse = http.post(
    `${baseURL}/auth/register`,
    JSON.stringify({ account, password, nickname }),
    jsonHeaders(),
  );
  if (registerResponse.status === 201) {
    return authUserFromResponse(registerResponse);
  }
  if (registerResponse.status !== 409) {
    fail(`register ${account} failed: ${registerResponse.status} ${registerResponse.body}`);
  }

  const loginResponse = http.post(
    `${baseURL}/auth/login`,
    JSON.stringify({ account, password }),
    jsonHeaders(),
  );
  if (loginResponse.status !== 200) {
    fail(`login ${account} failed: ${loginResponse.status} ${loginResponse.body}`);
  }
  return authUserFromResponse(loginResponse);
}

function createRoom(token, mediaItemID) {
  const response = http.post(
    `${baseURL}/rooms`,
    JSON.stringify({ mediaItemId: mediaItemID }),
    authJSONHeaders(token),
  );
  if (response.status !== 201) {
    fail(`create room failed: ${response.status} ${response.body}`);
  }
  const body = response.json();
  return body.data.room.roomCode;
}

function joinRoom(token, roomCode) {
  const response = http.post(`${baseURL}/rooms/${roomCode}/join`, null, authJSONHeaders(token));
  if (response.status !== 200) {
    fail(`join room ${roomCode} failed: ${response.status} ${response.body}`);
  }
}

function authUserFromResponse(response) {
  const body = response.json();
  return {
    userId: body.data.user.id,
    token: body.data.accessToken,
    deviceId: `load-device-${account}`,
  };
}

function nextControl(roomId, userId, seq, index) {
  const positionMs = (index + 1) * 1000;
  if (controlPattern === 'seek') {
    return {
      type: 'seek',
      payload: {
        roomId,
        userId,
        positionMs,
        seq,
        requestId: `load-seek-${__VU}-${index}`,
      },
    };
  }
  if (index % 5 === 2) {
    return {
      type: 'seek',
      payload: {
        roomId,
        userId,
        positionMs,
        seq,
        requestId: `load-seek-${__VU}-${index}`,
      },
    };
  }
  if (index % 2 === 0) {
    return {
      type: 'play',
      payload: {
        roomId,
        userId,
        positionMs,
        seq,
        requestId: `load-play-${__VU}-${index}`,
      },
    };
  }
  return {
    type: 'pause',
    payload: {
      roomId,
      userId,
      positionMs,
      seq,
      requestId: `load-pause-${__VU}-${index}`,
    },
  };
}

function jsonHeaders() {
  return {
    headers: {
      'Content-Type': 'application/json',
    },
  };
}

function authJSONHeaders(token) {
  return {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${token}`,
    },
  };
}
