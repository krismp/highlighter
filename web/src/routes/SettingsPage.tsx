import { SettingsCard } from "@/components/SettingsCard";
import { ToolsStatus } from "@/components/ToolsBanner";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";

export function SettingsPage() {
  return (
    <div className="mx-auto grid max-w-3xl gap-6">
      <SettingsCard />
      <Card>
        <CardHeader><CardTitle>Bundled tools</CardTitle></CardHeader>
        <CardContent><ToolsStatus /></CardContent>
      </Card>
    </div>
  );
}
