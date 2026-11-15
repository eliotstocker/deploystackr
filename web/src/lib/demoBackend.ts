import type {
    AuthMeResponse,
    AuthProvidersResponse,
    AuthUser,
    ChangelogDiff,
    ClusterStatus,
    DeploymentEvent,
    DevPersona,
    PromotionRequest,
    PromotionResult,
    ServiceResponse,
    VersionInfo
} from '../types';

const DEMO_PERSONAS: DevPersona[] = [
    {
        id: 'platform-lead',
        name: 'Alex Rivera',
        role: 'Platform Lead',
        description: 'Full promotion access across dev, staging, preprod, and production'
    },
    {
        id: 'app-dev',
        name: 'Jordan Chen',
        role: 'Application Developer',
        description: 'Can promote to dev and staging; production requires platform team'
    },
    {
        id: 'viewer',
        name: 'Sam Taylor',
        role: 'Read-Only Observer',
        description: 'Can inspect versions, diffs, and audit history without promoting'
    }
];

const PERSONA_USERS: Record<string, AuthUser> = {
    'platform-lead': {
        id: 'demo-alex',
        username: 'arivera',
        name: 'Alex Rivera',
        email: 'alex@acme.dev',
        provider: 'github',
        teams: ['org/platform-team', 'org/billing-team', 'org/auth-team'],
        groups: ['platform-engineers@acme.dev'],
        repos: {'org/gitops-manifests': 'admin'},
        deploy_repos: ['org/gitops-manifests']
    },
    'app-dev': {
        id: 'demo-jordan',
        username: 'jchen',
        name: 'Jordan Chen',
        email: 'jordan@acme.dev',
        provider: 'github',
        teams: ['org/billing-team'],
        groups: ['app-engineers@acme.dev'],
        repos: {'org/gitops-manifests': 'write'},
        deploy_repos: ['org/gitops-manifests']
    },
    'viewer': {
        id: 'demo-sam',
        username: 'staylor',
        name: 'Sam Taylor',
        email: 'sam@acme.dev',
        provider: 'github',
        teams: [],
        groups: ['observers@acme.dev'],
        repos: {'org/gitops-manifests': 'read'},
        deploy_repos: ['org/gitops-manifests']
    }
};

type DemoState = {
    user: AuthUser | null;
    services: ServiceResponse[];
    versions: Record<string, VersionInfo[]>;
    history: Record<string, Record<string, DeploymentEvent[]>>;
    prs: Record<string, Record<string, number[]>>;
    nextPrNumber: number;
};

function ago(minutes: number): string {
    return new Date(Date.now() - minutes * 60 * 1000).toISOString();
}

function shortSha(seed: string): string {
    let hash = 0;
    for(let i = 0; i < seed.length; i++) {
        hash = ((hash << 5) - hash + seed.charCodeAt(i)) | 0;
    }
    return Math.abs(hash).toString(16).padStart(7, '0').slice(0, 7);
}

function createInitialState(): DemoState {
    const billingHistory: Record<string, DeploymentEvent[]> = {
        production: [
            {
                service: 'billing-service',
                environment: 'production',
                version: 'v2.4.0',
                commit_sha: '8f3a19c',
                commit_message: 'chore(production): promote billing-service to v2.4.0',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                pr_number: 141,
                deployed_at: ago(420),
                is_active: true
            },
            {
                service: 'billing-service',
                environment: 'production',
                version: 'v2.3.9',
                commit_sha: '4c21b0e',
                commit_message: 'chore(production): promote billing-service to v2.3.9',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                pr_number: 138,
                deployed_at: ago(2880),
                retired_at: ago(420),
                active_duration: '1d 17h',
                is_active: false
            },
            {
                service: 'billing-service',
                environment: 'production',
                version: 'v2.3.8',
                commit_sha: '1a9d44f',
                commit_message: 'chore(production): promote billing-service to v2.3.8',
                author_name: 'Jordan Chen',
                author_email: 'jordan@acme.dev',
                pr_number: 134,
                deployed_at: ago(7200),
                retired_at: ago(2880),
                active_duration: '3d 0h',
                is_active: false
            }
        ],
        preprod: [
            {
                service: 'billing-service',
                environment: 'preprod',
                version: 'v2.4.1',
                commit_sha: 'e71c20b',
                commit_message: 'chore(preprod): promote billing-service to v2.4.1',
                author_name: 'Jordan Chen',
                author_email: 'jordan@acme.dev',
                deployed_at: ago(95),
                is_active: true
            },
            {
                service: 'billing-service',
                environment: 'preprod',
                version: 'v2.4.0',
                commit_sha: '3b8a11d',
                commit_message: 'chore(preprod): promote billing-service to v2.4.0',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                deployed_at: ago(600),
                retired_at: ago(95),
                active_duration: '8h 25m',
                is_active: false
            }
        ],
        staging: [
            {
                service: 'billing-service',
                environment: 'staging',
                version: 'v2.4.2',
                commit_sha: 'c42d99a',
                commit_message: 'chore(staging): promote billing-service to v2.4.2',
                author_name: 'Jordan Chen',
                author_email: 'jordan@acme.dev',
                deployed_at: ago(28),
                is_active: true
            },
            {
                service: 'billing-service',
                environment: 'staging',
                version: 'v2.4.1',
                commit_sha: '7a02e1c',
                commit_message: 'chore(staging): promote billing-service to v2.4.1',
                author_name: 'Jordan Chen',
                author_email: 'jordan@acme.dev',
                deployed_at: ago(180),
                retired_at: ago(28),
                active_duration: '2h 32m',
                is_active: false
            }
        ]
    };

    const authHistory: Record<string, DeploymentEvent[]> = {
        production: [
            {
                service: 'auth-service',
                environment: 'production',
                version: 'v1.8.2',
                commit_sha: '9d4e21a',
                commit_message: 'chore(production): promote auth-service to v1.8.2',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                pr_number: 139,
                deployed_at: ago(1440),
                is_active: true
            },
            {
                service: 'auth-service',
                environment: 'production',
                version: 'v1.8.1',
                commit_sha: '5f1b88c',
                commit_message: 'chore(production): promote auth-service to v1.8.1',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                pr_number: 131,
                deployed_at: ago(5760),
                retired_at: ago(1440),
                active_duration: '3d 0h',
                is_active: false
            }
        ],
        staging: [
            {
                service: 'auth-service',
                environment: 'staging',
                version: 'v1.8.2',
                commit_sha: '2b7c10e',
                commit_message: 'chore(staging): promote auth-service to v1.8.2',
                author_name: 'Jordan Chen',
                author_email: 'jordan@acme.dev',
                deployed_at: ago(1620),
                is_active: true
            }
        ]
    };

    const notifHistory: Record<string, DeploymentEvent[]> = {
        production: [
            {
                service: 'notification-service',
                environment: 'production',
                version: 'v3.0.4',
                commit_sha: '6a8c32f',
                commit_message: 'chore(production): promote notification-service to v3.0.4',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                pr_number: 136,
                deployed_at: ago(2100),
                is_active: true
            }
        ],
        staging: [
            {
                service: 'notification-service',
                environment: 'staging',
                version: 'v3.1.0',
                commit_sha: 'b19f44d',
                commit_message: 'chore(staging): promote notification-service to v3.1.0',
                author_name: 'Jordan Chen',
                author_email: 'jordan@acme.dev',
                deployed_at: ago(75),
                is_active: true
            }
        ]
    };

    const gatewayHistory: Record<string, DeploymentEvent[]> = {
        production: [
            {
                service: 'api-gateway',
                environment: 'production',
                version: 'v4.2.0',
                commit_sha: 'd30a71b',
                commit_message: 'chore(production): promote api-gateway to v4.2.0',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                pr_number: 140,
                deployed_at: ago(890),
                is_active: true
            }
        ],
        preprod: [
            {
                service: 'api-gateway',
                environment: 'preprod',
                version: 'v4.2.1',
                commit_sha: 'a54e82c',
                commit_message: 'chore(preprod): promote api-gateway to v4.2.1',
                author_name: 'Alex Rivera',
                author_email: 'alex@acme.dev',
                deployed_at: ago(210),
                is_active: true
            }
        ],
        staging: [
            {
                service: 'api-gateway',
                environment: 'staging',
                version: 'v4.2.1',
                commit_sha: 'f09c12e',
                commit_message: 'chore(staging): promote api-gateway to v4.2.1',
                author_name: 'Jordan Chen',
                author_email: 'jordan@acme.dev',
                deployed_at: ago(300),
                is_active: true
            }
        ]
    };

    const services: ServiceResponse[] = [
        {
            service: 'billing-service',
            display_name: 'Billing Service',
            icon: 'payments',
            gitops_repo: 'org/gitops-manifests',
            deploy_repos: ['org/gitops-manifests'],
            source: {type: 'oci', repository: 'ghcr.io/org/billing-service', filter: '^v[0-9]+\\.[0-9]+\\.[0-9]+$'},
            context: {repo: 'org/billing-service', changelog_strategy: 'git_diff'},
            environment_priority: ['production', 'preprod', 'staging'],
            latest_version: 'v2.4.2',
            environments: {
                production: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v2.4.0',
                    last_deployed: ago(420),
                    deployed_by: 'Alex Rivera',
                    commit_sha: '8f3a19c',
                    pr_number: 141,
                    write_strategy: 'pull_request',
                    target_file: 'apps/production/billing/deployment.json',
                    target_path: '$.spec.template.spec.containers[0].image',
                    recent_events: billingHistory.production,
                    cluster_status: {status: 'ready', ready_replicas: 3, updated_replicas: 3, desired_replicas: 3, current_version: 'v2.4.0', message: 'Healthy'}
                },
                preprod: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v2.4.1',
                    last_deployed: ago(95),
                    deployed_by: 'Jordan Chen',
                    commit_sha: 'e71c20b',
                    write_strategy: 'direct',
                    target_file: 'apps/preprod/billing/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: billingHistory.preprod,
                    cluster_status: {status: 'ready', ready_replicas: 2, updated_replicas: 2, desired_replicas: 2, current_version: 'v2.4.1', message: 'Healthy'}
                },
                staging: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v2.4.2',
                    last_deployed: ago(28),
                    deployed_by: 'Jordan Chen',
                    commit_sha: 'c42d99a',
                    write_strategy: 'direct',
                    target_file: 'apps/staging/billing/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: billingHistory.staging,
                    cluster_status: {status: 'ready', ready_replicas: 2, updated_replicas: 2, desired_replicas: 2, current_version: 'v2.4.2', message: 'Healthy'}
                }
            }
        },
        {
            service: 'auth-service',
            display_name: 'Auth Service',
            icon: 'lock',
            gitops_repo: 'org/gitops-manifests',
            deploy_repos: ['org/gitops-manifests'],
            source: {type: 'github_releases', repository: 'org/auth-service', filter: '^v[0-9]+\\.[0-9]+\\.[0-9]+$'},
            context: {repo: 'org/auth-service', changelog_strategy: 'git_diff'},
            environment_priority: ['production', 'staging'],
            latest_version: 'v1.8.2',
            environments: {
                production: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v1.8.2',
                    last_deployed: ago(1440),
                    deployed_by: 'Alex Rivera',
                    commit_sha: '9d4e21a',
                    pr_number: 139,
                    write_strategy: 'pull_request',
                    target_file: 'apps/production/auth/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: authHistory.production,
                    cluster_status: {status: 'ready', ready_replicas: 3, updated_replicas: 3, desired_replicas: 3, current_version: 'v1.8.2', message: 'Healthy'}
                },
                staging: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v1.8.2',
                    last_deployed: ago(1620),
                    deployed_by: 'Jordan Chen',
                    commit_sha: '2b7c10e',
                    write_strategy: 'direct',
                    target_file: 'apps/staging/auth/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: authHistory.staging,
                    cluster_status: {status: 'ready', ready_replicas: 2, updated_replicas: 2, desired_replicas: 2, current_version: 'v1.8.2', message: 'Healthy'}
                }
            }
        },
        {
            service: 'notification-service',
            display_name: 'Notification Service',
            icon: 'notifications',
            gitops_repo: 'org/gitops-manifests',
            deploy_repos: ['org/gitops-manifests'],
            source: {type: 'git_tags', repository: 'org/notification-service', filter: '^v[0-9]+\\.[0-9]+\\.[0-9]+$'},
            context: {repo: 'org/notification-service', changelog_strategy: 'git_diff'},
            environment_priority: ['production', 'staging'],
            latest_version: 'v3.1.0',
            environments: {
                production: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v3.0.4',
                    last_deployed: ago(2100),
                    deployed_by: 'Alex Rivera',
                    commit_sha: '6a8c32f',
                    pr_number: 136,
                    write_strategy: 'pull_request',
                    target_file: 'apps/production/notifications/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: notifHistory.production,
                    cluster_status: {status: 'ready', ready_replicas: 2, updated_replicas: 2, desired_replicas: 2, current_version: 'v3.0.4', message: 'Healthy'}
                },
                staging: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v3.1.0',
                    last_deployed: ago(75),
                    deployed_by: 'Jordan Chen',
                    commit_sha: 'b19f44d',
                    write_strategy: 'direct',
                    target_file: 'apps/staging/notifications/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: notifHistory.staging,
                    cluster_status: {status: 'ready', ready_replicas: 1, updated_replicas: 1, desired_replicas: 1, current_version: 'v3.1.0', message: 'Healthy'}
                }
            }
        },
        {
            service: 'api-gateway',
            display_name: 'API Gateway',
            icon: 'dns',
            gitops_repo: 'org/gitops-manifests',
            deploy_repos: ['org/gitops-manifests'],
            source: {type: 'oci', repository: 'ghcr.io/org/api-gateway', filter: '^v[0-9]+\\.[0-9]+\\.[0-9]+$'},
            context: {repo: 'org/api-gateway', changelog_strategy: 'git_diff'},
            environment_priority: ['production', 'preprod', 'staging'],
            latest_version: 'v4.2.1',
            environments: {
                production: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v4.2.0',
                    last_deployed: ago(890),
                    deployed_by: 'Alex Rivera',
                    commit_sha: 'd30a71b',
                    pr_number: 140,
                    write_strategy: 'pull_request',
                    target_file: 'apps/production/gateway/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: gatewayHistory.production,
                    cluster_status: {status: 'ready', ready_replicas: 4, updated_replicas: 4, desired_replicas: 4, current_version: 'v4.2.0', message: 'Healthy'}
                },
                preprod: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v4.2.1',
                    last_deployed: ago(210),
                    deployed_by: 'Alex Rivera',
                    commit_sha: 'a54e82c',
                    write_strategy: 'direct',
                    target_file: 'apps/preprod/gateway/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: gatewayHistory.preprod,
                    cluster_status: {status: 'ready', ready_replicas: 2, updated_replicas: 2, desired_replicas: 2, current_version: 'v4.2.1', message: 'Healthy'}
                },
                staging: {
                    repo: 'org/gitops-manifests',
                    active_version: 'v4.2.1',
                    last_deployed: ago(300),
                    deployed_by: 'Jordan Chen',
                    commit_sha: 'f09c12e',
                    write_strategy: 'direct',
                    target_file: 'apps/staging/gateway/values.yaml',
                    target_path: '$.image.tag',
                    recent_events: gatewayHistory.staging,
                    cluster_status: {status: 'ready', ready_replicas: 2, updated_replicas: 2, desired_replicas: 2, current_version: 'v4.2.1', message: 'Healthy'}
                }
            }
        }
    ];

    const versions: Record<string, VersionInfo[]> = {
        'billing-service': [
            {
                version: 'v2.4.2',
                released_at: ago(35),
                description: 'Fix idempotency key collision on retryable invoice webhooks',
                changelog: [
                    '### Bug Fixes',
                    '- Fix idempotency key collision on retryable invoice webhooks (#145)',
                    '- Guard against duplicate webhook delivery during ledger flush (#144)',
                    '',
                    '### Maintenance',
                    '- Add regression tests for concurrent invoice finalization'
                ].join('\n')
            },
            {
                version: 'v2.4.1',
                released_at: ago(190),
                description: 'Add Stripe SEPA Direct Debit settlement reconciliation',
                changelog: [
                    '### Features',
                    '- Add Stripe SEPA Direct Debit settlement reconciliation (#143)',
                    '- Emit Prometheus counter for pending bank mandate verifications (#142)',
                    '',
                    '### Bug Fixes',
                    '- Normalize EUR cent rounding on partial credit notes'
                ].join('\n')
            },
            {
                version: 'v2.4.0',
                released_at: ago(650),
                description: 'Support multi-currency tax rounding and usage-based tiers',
                changelog: [
                    '### Features',
                    '- Support multi-currency tax rounding across 28 EU VAT regions (#141)',
                    '- Add metered usage-based pricing tiers with soft-cap alerts (#140)',
                    '',
                    '### Performance',
                    '- Batch Stripe usage record uploads every 30s (#139)'
                ].join('\n')
            },
            {
                version: 'v2.3.9',
                released_at: ago(3000),
                description: 'Optimize ledger query batching for monthly close',
                changelog: [
                    '### Performance',
                    '- Optimize ledger query batching for monthly close (#138)',
                    '- Reduce Postgres connection pool contention under peak billing runs'
                ].join('\n')
            },
            {
                version: 'v2.3.8',
                released_at: ago(7300),
                description: 'Upgrade Go runtime to 1.24 and patch TLS cipher defaults',
                changelog: [
                    '### Security & Runtime',
                    '- Upgrade Go runtime to 1.24 and enforce TLS 1.3 cipher defaults (#134)',
                    '- Update stripe-go SDK to v81.2.0'
                ].join('\n')
            }
        ],
        'auth-service': [
            {
                version: 'v1.8.2',
                released_at: ago(1700),
                description: 'Enforce PKCE S256 on all public OAuth2 clients',
                changelog: [
                    '### Security',
                    '- Enforce PKCE S256 on all public OAuth2 clients (#139)',
                    '- Reject legacy implicit-flow redirect URIs with clear error codes (#137)'
                ].join('\n')
            },
            {
                version: 'v1.8.1',
                released_at: ago(5900),
                description: 'Add JWKS key rotation grace window',
                changelog: [
                    '### Features',
                    '- Add 24h JWKS key rotation grace window for zero-downtime key rollover (#131)',
                    '',
                    '### Bug Fixes',
                    '- Preserve `amr` claim when refreshing SSO sessions'
                ].join('\n')
            },
            {
                version: 'v1.8.0',
                released_at: ago(11000),
                description: 'Support WebAuthn passkey step-up authentication',
                changelog: [
                    '### Features',
                    '- Support WebAuthn passkey step-up authentication for production promotions',
                    '- Add FIDO2 hardware key attestation metadata validation'
                ].join('\n')
            }
        ],
        'notification-service': [
            {
                version: 'v3.1.0',
                released_at: ago(90),
                description: 'Add Slack Block Kit templates and per-tenant rate limiter',
                changelog: [
                    '### Features',
                    '- Add Slack Block Kit templates for promotion & rollback alerts (#147)',
                    '- Add token-bucket per-tenant notification rate limiter (#146)',
                    '',
                    '### Bug Fixes',
                    '- Truncate oversized commit summaries in PagerDuty payloads'
                ].join('\n')
            },
            {
                version: 'v3.0.4',
                released_at: ago(2300),
                description: 'Fix exponential backoff jitter for SES bounce handler',
                changelog: [
                    '### Bug Fixes',
                    '- Fix exponential backoff jitter for AWS SES bounce handler (#136)',
                    '- Dead-letter malformed webhook templates instead of retrying'
                ].join('\n')
            },
            {
                version: 'v3.0.3',
                released_at: ago(6100),
                description: 'Reduce worker memory footprint during digest fanout',
                changelog: [
                    '### Performance',
                    '- Stream digest recipients in chunks of 500 to cut worker RSS by 45%'
                ].join('\n')
            }
        ],
        'api-gateway': [
            {
                version: 'v4.2.1',
                released_at: ago(320),
                description: 'Preserve traceparent header across gRPC-Web transcoding',
                changelog: [
                    '### Bug Fixes',
                    '- Preserve W3C `traceparent` header across gRPC-Web transcoding (#148)',
                    '- Fix idle HTTP/2 upstream keepalive timeout on slow healthchecks'
                ].join('\n')
            },
            {
                version: 'v4.2.0',
                released_at: ago(950),
                description: 'Add adaptive circuit breaker and per-route timeout budgets',
                changelog: [
                    '### Features',
                    '- Add adaptive circuit breaker with automatic half-open probing (#140)',
                    '- Support per-route deadline propagation and timeout budgets'
                ].join('\n')
            },
            {
                version: 'v4.1.9',
                released_at: ago(4200),
                description: 'Fix Brotli compression flush on streaming SSE responses',
                changelog: [
                    '### Bug Fixes',
                    '- Bypass Brotli buffering for `text/event-stream` SSE endpoints'
                ].join('\n')
            }
        ]
    };

    const history: Record<string, Record<string, DeploymentEvent[]>> = {
        'billing-service': billingHistory,
        'auth-service': authHistory,
        'notification-service': notifHistory,
        'api-gateway': gatewayHistory
    };

    const prs: Record<string, Record<string, number[]>> = {
        'billing-service': {'v2.4.2': [144, 145], 'v2.4.1': [142, 143], 'v2.4.0': [139, 140, 141]},
        'auth-service': {'v1.8.2': [137, 139], 'v1.8.1': [131]},
        'notification-service': {'v3.1.0': [146, 147], 'v3.0.4': [136]},
        'api-gateway': {'v4.2.1': [148], 'v4.2.0': [140]}
    };

    return {
        user: PERSONA_USERS['platform-lead'],
        services,
        versions,
        history,
        prs,
        nextPrNumber: 150
    };
}

const state: DemoState = createInitialState();

function computePermissions(svc: ServiceResponse, user: AuthUser | null): Record<string, {can_deploy: boolean; reason?: string}> {
    const perms: Record<string, {can_deploy: boolean; reason?: string}> = {};
    for(const envName of Object.keys(svc.environments)) {
        if(!user) {
            perms[envName] = {can_deploy: false, reason: 'Authentication required'};
            continue;
        }
        const repoPerm = user.repos['org/gitops-manifests'];
        if(repoPerm === 'read') {
            perms[envName] = {can_deploy: false, reason: 'Requires write access to GitOps repository'};
            continue;
        }
        if(envName === 'production' && !user.teams.includes('org/platform-team')) {
            perms[envName] = {can_deploy: false, reason: 'Production requires org/platform-team membership'};
            continue;
        }
        perms[envName] = {can_deploy: true};
    }
    return perms;
}

function computeTopEnvStatus(svc: ServiceResponse) {
    const priority = svc.environment_priority || Object.keys(svc.environments);
    const topEnv = priority.find(e => svc.environments[e]) || Object.keys(svc.environments)[0];
    if(!topEnv) return undefined;
    const envView = svc.environments[topEnv];
    const active = envView.active_version || 'unknown';
    const latest = svc.latest_version || '';
    const svcVersions = state.versions[svc.service] || [];
    const activeIdx = svcVersions.findIndex(v => v.version === active);
    const versionsBehind = activeIdx > -1 ? activeIdx : (active === latest ? 0 : 1);
    return {
        environment: topEnv,
        active_version: active,
        latest_version: latest,
        versions_behind: versionsBehind,
        is_up_to_date: active === latest,
        pending_pr: envView.pending_pr
    };
}

function cloneService(svc: ServiceResponse): ServiceResponse {
    const copy: ServiceResponse = JSON.parse(JSON.stringify(svc));
    copy.user_permissions = computePermissions(copy, state.user);
    copy.top_env_status = computeTopEnvStatus(copy);
    return copy;
}

function scheduleRollout(svc: ServiceResponse, envName: string, targetVersion: string) {
    const env = svc.environments[envName];
    if(!env) return;

    const desired = env.cluster_status?.desired_replicas || 2;
    env.cluster_status = {
        status: 'pending',
        ready_replicas: 0,
        updated_replicas: 0,
        desired_replicas: desired,
        current_version: env.active_version,
        message: 'Waiting for GitOps rollout to start...'
    };

    //transition from pending to deploying (progressing)
    setTimeout(() => {
        if(!env.cluster_status || env.cluster_status.status !== 'pending') return;
        const partial = Math.max(1, Math.floor(desired / 2));
        env.cluster_status = {
            status: 'progressing',
            ready_replicas: partial,
            updated_replicas: partial,
            desired_replicas: desired,
            current_version: targetVersion,
            message: `Rolling out pods (${partial}/${desired} ready)...`
        };
    }, 2200);

    //transition from deploying to ready (done)
    setTimeout(() => {
        if(!env.cluster_status || env.cluster_status.status !== 'progressing') return;
        env.cluster_status = {
            status: 'ready',
            ready_replicas: desired,
            updated_replicas: desired,
            desired_replicas: desired,
            current_version: targetVersion,
            message: 'Healthy'
        };
    }, 5200);
}

function applyGitCommit(
    svc: ServiceResponse,
    envName: string,
    targetVersion: string,
    userName: string,
    userEmail: string,
    prNumber?: number
): string {
    const env = svc.environments[envName];
    const sha = shortSha(`${svc.service}:${envName}:${targetVersion}:${Date.now()}`);
    const now = new Date().toISOString();

    env.active_version = targetVersion;
    env.last_deployed = now;
    env.deployed_by = userName;
    env.commit_sha = sha;
    if(prNumber) {
        env.pr_number = prNumber;
    }

    const svcHist = state.history[svc.service] || (state.history[svc.service] = {});
    const envHist = svcHist[envName] || (svcHist[envName] = []);
    if(envHist.length > 0 && envHist[0].is_active) {
        envHist[0].is_active = false;
        envHist[0].retired_at = now;
        envHist[0].active_duration = 'just now';
    }

    envHist.unshift({
        service: svc.service,
        environment: envName,
        version: targetVersion,
        commit_sha: sha,
        commit_message: `chore(${envName}): promote ${svc.service} to ${targetVersion}`,
        author_name: userName,
        author_email: userEmail,
        pr_number: prNumber,
        deployed_at: now,
        is_active: true
    });
    env.recent_events = envHist;

    return sha;
}

export async function demoFetchAuthMe(): Promise<AuthMeResponse> {
    if(!state.user) {
        return {authenticated: false};
    }
    return {authenticated: true, user: state.user};
}

export async function demoFetchAuthProviders(): Promise<AuthProvidersResponse> {
    return {
        providers: ['dev'],
        dev_mode: true,
        personas: DEMO_PERSONAS
    };
}

export async function demoLoginDev(personaId: string): Promise<AuthMeResponse> {
    const user = PERSONA_USERS[personaId] || PERSONA_USERS['platform-lead'];
    state.user = user;
    return {authenticated: true, user};
}

export async function demoLogoutUser(): Promise<void> {
    state.user = null;
}

export async function demoFetchServices(): Promise<ServiceResponse[]> {
    return state.services.map(cloneService);
}

export async function demoFetchVersions(service: string): Promise<VersionInfo[]> {
    return JSON.parse(JSON.stringify(state.versions[service] || []));
}

export async function demoFetchHistory(service: string): Promise<Record<string, DeploymentEvent[]>> {
    return JSON.parse(JSON.stringify(state.history[service] || {}));
}

export async function demoFetchDiff(
    service: string,
    fromVersion?: string,
    toVersion?: string
): Promise<ChangelogDiff> {
    const from = fromVersion || 'v0.0.0';
    const to = toVersion || 'latest';
    const vers = state.versions[service] || [];
    const fromIdx = vers.findIndex(v => v.version === from);
    const toIdx = vers.findIndex(v => v.version === to);
    const isRollback = fromIdx > -1 && toIdx > -1 && toIdx > fromIdx;

    const svc = state.services.find(s => s.service === service);
    const targetVerInfo = vers.find(v => v.version === to);
    const commits = [
        {
            sha: shortSha(`${service}:${to}:1`),
            author_name: 'Jordan Chen',
            author_email: 'jordan@acme.dev',
            timestamp: ago(45),
            subject: targetVerInfo?.description || `feat(${service}): release ${to}`,
            pr_number: 145
        },
        {
            sha: shortSha(`${service}:${to}:2`),
            author_name: 'Alex Rivera',
            author_email: 'alex@acme.dev',
            timestamp: ago(110),
            subject: `test(${service}): add regression coverage for ${to}`,
            pr_number: 144
        }
    ];

    const targetFile = svc?.environments?.staging?.target_file || `apps/staging/${service}/values.yaml`;
    const rawDiff = [
        `--- a/${targetFile}`,
        `+++ b/${targetFile}`,
        `@@ -12,4 +12,4 @@`,
        ` image:`,
        `   repository: ${svc?.source.repository || service}`,
        `-  tag: ${from}`,
        `+  tag: ${to}`
    ].join('\n');

    return {
        from_version: from,
        to_version: to,
        commits,
        diff_stat: `1 file changed, 1 insertion(+), 1 deletion(-)`,
        raw_diff: rawDiff,
        is_rollback: isRollback
    };
}

export async function demoPromote(payload: PromotionRequest): Promise<PromotionResult> {
    const svc = state.services.find(s => s.service === payload.service);
    if(!svc) {
        throw new Error(`Service "${payload.service}" not found`);
    }
    const env = svc.environments[payload.environment];
    if(!env) {
        throw new Error(`Environment "${payload.environment}" not found`);
    }

    const perms = computePermissions(svc, state.user);
    if(perms[payload.environment] && !perms[payload.environment].can_deploy) {
        throw new Error(perms[payload.environment].reason || 'Permission denied');
    }

    const mode = payload.mode || env.write_strategy || 'direct';
    const oldVersion = env.active_version;
    const userName = state.user?.name || payload.user || 'Alex Rivera';
    const userEmail = state.user?.email || payload.email || 'alex@acme.dev';

    if(mode === 'pull_request') {
        const prNum = state.nextPrNumber++;
        const branch = `deploystackr/${svc.service}-${payload.environment}-${payload.target_version}`;
        const prUrl = `https://github.com/org/gitops-manifests/pull/${prNum}`;
        env.pending_pr = {
            service: svc.service,
            environment: payload.environment,
            target_version: payload.target_version,
            branch,
            pr_number: prNum,
            pr_url: prUrl,
            user: userName,
            created_at: new Date().toISOString()
        };

        //auto-merge simulated PR after 6s so visitors can observe the full PR -> pending -> deploying -> ready lifecycle
        setTimeout(() => {
            if(!env.pending_pr || env.pending_pr.pr_number !== prNum) return;
            delete env.pending_pr;
            applyGitCommit(svc, payload.environment, payload.target_version, userName, userEmail, prNum);
            scheduleRollout(svc, payload.environment, payload.target_version);
        }, 6000);

        return {
            success: true,
            mode: 'pull_request',
            branch,
            pr_number: prNum,
            pr_url: prUrl,
            message: `Opened PR #${prNum} to promote ${svc.service} to ${payload.target_version}`,
            target_file: env.target_file,
            old_version: oldVersion,
            new_version: payload.target_version
        };
    }

    const sha = applyGitCommit(svc, payload.environment, payload.target_version, userName, userEmail);
    scheduleRollout(svc, payload.environment, payload.target_version);

    return {
        success: true,
        mode: 'direct',
        commit_sha: sha,
        message: `Committed ${payload.target_version} to ${env.target_file}`,
        target_file: env.target_file,
        old_version: oldVersion,
        new_version: payload.target_version
    };
}

export async function demoTriggerSync(): Promise<{status: string; message: string}> {
    return {status: 'ok', message: 'Demo GitOps state refreshed'};
}

export async function demoFetchServiceStatus(
    service: string,
    env?: string
): Promise<Record<string, ClusterStatus>> {
    const svc = state.services.find(s => s.service === service);
    if(!svc) return {};
    const out: Record<string, ClusterStatus> = {};
    for(const [envName, envView] of Object.entries(svc.environments)) {
        if(env && envName !== env) continue;
        if(envView.cluster_status) {
            out[envName] = {...envView.cluster_status};
        }
    }
    return out;
}

export async function demoFetchReleasePRs(service: string): Promise<Record<string, number[]>> {
    return JSON.parse(JSON.stringify(state.prs[service] || {}));
}
