/** Публичные метки текущего frontend-артефакта, задаваемые единожды при сборке. */
export interface AppBuildInfo {
  version: string;
  commit: string;
  buildTime: string;
}

declare const __APP_BUILD__: AppBuildInfo;
declare const __CLIENT_TELEMETRY_ENABLED__: boolean;

export const appBuild: Readonly<AppBuildInfo> = Object.freeze(__APP_BUILD__);
export const clientTelemetryEnabled = __CLIENT_TELEMETRY_ENABLED__;
