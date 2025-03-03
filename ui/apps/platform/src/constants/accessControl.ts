/* constants specific to Roles */
import { ResourceName } from '../types/roleResources';

export const NO_ACCESS = 'NO_ACCESS';
export const READ_ACCESS = 'READ_ACCESS';
export const READ_WRITE_ACCESS = 'READ_WRITE_ACCESS';

export type AccessLevel = 'NO_ACCESS' | 'READ_ACCESS' | 'READ_WRITE_ACCESS';

const defaultRoles = {
    Admin: true,
    Analyst: true,
    'Continuous Integration': true,
    None: true,
    'Scope Manager': true,
    'Sensor Creator': true,
    'Vulnerability Management Approver': true,
    'Vulnerability Management Requester': true,
    'Vulnerability Report Creator': true,
};

export function getIsDefaultRoleName(name: string): boolean {
    return Boolean(defaultRoles[name]);
}

export const authProviderLabels = {
    auth0: 'Auth0',
    oidc: 'OpenID Connect',
    saml: 'SAML 2.0',
    userpki: 'User Certificates',
    iap: 'Google IAP',
    openshift: 'OpenShift Auth',
};

export const oidcCallbackModes = [
    {
        label: 'Auto-select (recommended)',
        value: 'auto',
    },
    {
        label: 'HTTP POST',
        value: 'post',
    },
    {
        label: 'Fragment',
        value: 'fragment',
    },
    {
        label: 'Query',
        value: 'query',
    },
];

// DEPRECATED, replaced by map for SAC above
export const oidcCallbackValues = {
    auto: 'Auto-select (recommended)',
    post: 'HTTP POST',
    fragment: 'Fragment',
    query: 'Query',
};

export const defaultMinimalReadAccessResources = [
    'Alert',
    'Cluster',
    // TODO: ROX-12750 Replace Config with Administration
    'Config',
    'Deployment',
    'Image',
    'Namespace',
    'NetworkPolicy',
    'NetworkGraph',
    'Node',
    'Policy',
    'Secret',
];

// Default to giving new roles read access to specific resources.
export const defaultNewRolePermissions = defaultMinimalReadAccessResources.reduce(
    (map: Record<string, AccessLevel>, resource) => {
        const newMap = map;
        newMap[resource] = READ_ACCESS;
        return newMap;
    },
    {}
);

export const defaultSelectedRole = {
    name: '',
    resourceToAccess: defaultNewRolePermissions,
};

// TODO: ROX-12750 update with new list of replaced/deprecated resources
export const resourceSubstitutions: Record<string, string[]> = {
    Access: ['AuthProvider', 'Group', 'Licenses', 'User'],
    DeploymentExtension: ['Indicator', 'NetworkBaseline', 'ProcessWhitelist', 'Risk'],
    Integration: [
        'APIToken',
        'BackupPlugins',
        'ImageIntegration',
        'Notifier',
        'SignatureIntegration',
    ],
    Image: ['ImageComponent'],
};

// TODO: ROX-12750 update with new list of replaced/deprecated resources
export const resourceRemovalReleaseVersions = new Map<ResourceName, string>([
    ['AllComments', '3.75'],
    ['ComplianceRuns', '3.75'],
    ['Config', '3.75'],
    ['DebugLogs', '3.75'],
    ['NetworkGraphConfig', '3.75'],
    ['ProbeUpload', '3.75'],
    ['ScannerBundle', '3.75'],
    ['ScannerDefinitions', '3.75'],
    ['SensorUpgradeConfig', '3.75'],
    ['ServiceIdentity', '3.75'],
    ['ClusterCVE', '3.74'],
]);

// TODO(ROX-11453): Remove this mapping once the old resources are fully deprecated.
export const replacedResourceMapping = new Map<ResourceName, string>([
    // TODO: ROX-12750 Remove AllComments, ComplianceRunSchedule, ComplianceRuns, Config, DebugLogs,
    // NetworkGraphConfig, ProbeUpload, ScannerBundle, ScannerDefinitions, SensorUpgradeConfig and ServiceIdentity.
    ['AllComments', 'Administration'],
    ['ComplianceRuns', 'Compliance'],
    ['Config', 'Administration'],
    ['DebugLogs', 'Administration'],
    ['NetworkGraphConfig', 'Administration'],
    ['ProbeUpload', 'Administration'],
    ['ScannerBundle', 'Administration'],
    ['ScannerDefinitions', 'Administration'],
    ['SensorUpgradeConfig', 'Administration'],
    ['ServiceIdentity', 'Administration'],
    ['ClusterCVE', 'Cluster'],
]);

export const deprecatedResourceRowStyle = { backgroundColor: 'rgb(255,250,205)' };
