import { Link } from "@tanstack/react-router";
import { ArrowLeft } from "lucide-react";
import { Button } from "@/components/ui/button";
import { PageHeader } from "@/components/page-header";
import { APIKeyForm } from "@/features/api-keys/api-key-form";

export function APIKeyNewPage() {
  return (
    <div className="mx-auto max-w-2xl">
      <Button variant="ghost" size="sm" asChild className="mb-4 -ml-2">
        <Link to="/api-keys">
          <ArrowLeft className="size-4" />
          API keys
        </Link>
      </Button>
      <PageHeader
        title="New API key"
        description="Mint a credential for agents, the CLI, or the MCP server. The plaintext shows once after creation."
      />
      <APIKeyForm />
    </div>
  );
}
