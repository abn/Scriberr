import { useCallback, useEffect, useState } from "react";
import { Server } from "lucide-react";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { useAuth } from "@/features/auth/hooks/useAuth";

interface SystemSettingsState {
	deployment_mode: "single_user" | "multi_user";
	startup_diarization_model: "none" | "pyannote" | "sortformer";
	can_manage: boolean;
}

export function SystemSettings() {
	const { getAuthHeaders } = useAuth();
	const [settings, setSettings] = useState<SystemSettingsState | null>(null);
	const [loading, setLoading] = useState(true);
	const [saving, setSaving] = useState(false);
	const [error, setError] = useState("");
	const [success, setSuccess] = useState("");

	const loadSettings = useCallback(async () => {
		try {
			const response = await fetch("/api/v1/system/settings", { headers: getAuthHeaders() });
			if (!response.ok) throw new Error("Failed to load global settings");
			setSettings(await response.json());
		} catch (loadError) {
			setError(loadError instanceof Error ? loadError.message : "Failed to load global settings");
		} finally {
			setLoading(false);
		}
	}, [getAuthHeaders]);

	useEffect(() => {
		loadSettings();
	}, [loadSettings]);

	const updateSettings = async (updates: Partial<SystemSettingsState>, successMessage: string) => {
		setSaving(true);
		setError("");
		setSuccess("");
		try {
			const response = await fetch("/api/v1/system/settings", {
				method: "PUT",
				headers: { "Content-Type": "application/json", ...getAuthHeaders() },
				body: JSON.stringify(updates),
			});
			const body = await response.json().catch(() => ({}));
			if (!response.ok) throw new Error(body.error || "Failed to save global settings");
			setSettings(body);
			setSuccess(successMessage);
		} catch (saveError) {
			setError(saveError instanceof Error ? saveError.message : "Failed to save global settings");
		} finally {
			setSaving(false);
		}
	};

	return (
		<div className="space-y-6">
			{error && <div className="bg-[var(--error)]/10 border border-[var(--error)]/20 rounded-lg p-3 text-sm text-[var(--error)]">{error}</div>}
			{success && <div className="bg-[var(--success-translucent)] border border-[var(--success-solid)]/20 rounded-lg p-3 text-sm text-[var(--success-solid)]">{success}</div>}

			<div className="bg-[var(--bg-main)]/50 border border-[var(--border-subtle)] rounded-[var(--radius-card)] p-4 sm:p-6 shadow-sm">
				<div className="flex items-center gap-2 mb-2">
					<Server className="h-5 w-5 text-[var(--brand-solid)]" />
					<h3 className="text-lg font-medium text-[var(--text-primary)]">Global Deployment Settings</h3>
				</div>
				<p className="text-sm text-[var(--text-secondary)] mb-6">
					These settings control process-wide GPU resources. Only the first registered account can change them.
				</p>

				{loading ? (
					<p className="text-sm text-[var(--text-secondary)]">Loading settings...</p>
				) : settings ? (
					<div className="space-y-5">
						<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3">
							<div>
								<Label htmlFor="deployment-mode">Deployment Mode</Label>
								<p className="text-sm text-[var(--text-secondary)] mt-1">
									Multi-user mode restricts global resident-model controls to the deployment owner and keeps user credentials request-scoped.
								</p>
							</div>
							<Select
								value={settings.deployment_mode}
								onValueChange={(deployment_mode) => updateSettings({ deployment_mode: deployment_mode as SystemSettingsState["deployment_mode"] }, "Deployment mode updated.")}
								disabled={saving || !settings.can_manage}
							>
								<SelectTrigger id="deployment-mode" className="w-full sm:w-56 bg-[var(--bg-main)] border-[var(--border-subtle)] text-[var(--text-primary)]">
									<SelectValue />
								</SelectTrigger>
								<SelectContent className="bg-[var(--bg-card)] border-[var(--border-subtle)] text-[var(--text-primary)]">
									<SelectItem value="single_user">Single user</SelectItem>
									<SelectItem value="multi_user">Multi-user</SelectItem>
								</SelectContent>
							</Select>
						</div>

						<div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-3 border-t border-[var(--border-subtle)] pt-5">
							<div>
								<Label htmlFor="global-startup-diarization-model">Load Diarization Model at Startup</Label>
								<p className="text-sm text-[var(--text-secondary)] mt-1">
									This is process-wide. In multi-user mode, startup PyAnnote requires a server-level HF_TOKEN.
								</p>
							</div>
							<Select
								value={settings.startup_diarization_model}
								onValueChange={(startup_diarization_model) => updateSettings({ startup_diarization_model: startup_diarization_model as SystemSettingsState["startup_diarization_model"] }, "Startup diarization model updated.")}
								disabled={saving || !settings.can_manage}
							>
								<SelectTrigger id="global-startup-diarization-model" className="w-full sm:w-56 bg-[var(--bg-main)] border-[var(--border-subtle)] text-[var(--text-primary)]">
									<SelectValue />
								</SelectTrigger>
								<SelectContent className="bg-[var(--bg-card)] border-[var(--border-subtle)] text-[var(--text-primary)]">
									<SelectItem value="none">None</SelectItem>
									<SelectItem value="pyannote">PyAnnote</SelectItem>
									<SelectItem value="sortformer">Sortformer</SelectItem>
								</SelectContent>
							</Select>
						</div>

						{!settings.can_manage && (
							<p className="text-sm text-[var(--text-secondary)] border-t border-[var(--border-subtle)] pt-4">
								These global settings are read-only for this account.
							</p>
						)}
					</div>
				) : null}
			</div>
		</div>
	);
}
