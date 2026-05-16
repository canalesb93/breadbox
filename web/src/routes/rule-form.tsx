import { Link, useNavigate, useParams } from "@tanstack/react-router";
import { Loader2 } from "lucide-react";
import { PageHeader } from "@/components/page-header";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { withMutationToast } from "@/lib/mutation-toast";
import {
  useCreateRule,
  useRule,
  useUpdateRule,
} from "@/api/queries/rules";
import { RuleForm, type RuleFormSubmit } from "@/features/rules/rule-form";

interface RuleFormPageProps {
  mode: "create" | "edit";
}

// RuleFormPage hosts the shared <RuleForm /> body for both /rules/new and
// /rules/$id/edit. In edit mode it fetches the rule, threads the initial
// values through, and updates on submit. In create mode it skips the fetch.
export function RuleFormPage({ mode }: RuleFormPageProps) {
  const navigate = useNavigate();
  const params = useParams({ strict: false }) as { id?: string };
  const id = mode === "edit" ? params.id : undefined;

  const ruleQuery = useRule(id);
  const createRule = useCreateRule();
  const updateRule = useUpdateRule();

  const isEdit = mode === "edit";
  const rule = isEdit ? ruleQuery.data : undefined;

  const goBack = () => {
    if (isEdit && rule) {
      navigate({ to: "/rules/$id", params: { id: rule.short_id } });
    } else {
      navigate({ to: "/rules" });
    }
  };

  const onSubmit = async (values: RuleFormSubmit) => {
    if (isEdit && rule) {
      const ok = await withMutationToast(
        () =>
          updateRule.mutateAsync({
            id: rule.short_id,
            input: {
              name: values.name,
              conditions: values.conditions,
              actions: values.actions,
              trigger: values.trigger,
              priority: values.priority,
            },
          }),
        { success: `Updated rule "${values.name}".` },
      );
      if (ok) navigate({ to: "/rules/$id", params: { id: rule.short_id } });
    } else {
      let created;
      try {
        created = await createRule.mutateAsync({
          name: values.name,
          conditions: values.conditions,
          actions: values.actions,
          trigger: values.trigger,
          priority: values.priority,
        });
      } catch (err) {
        await withMutationToast(() => Promise.reject(err), {
          success: "",
          error: "Failed to create rule.",
        });
        return;
      }
      // Toast + redirect to the new rule's detail page.
      await withMutationToast(() => Promise.resolve(), {
        success: `Created rule "${created.name}".`,
      });
      navigate({ to: "/rules/$id", params: { id: created.short_id } });
    }
  };

  if (isEdit && ruleQuery.isLoading) {
    return (
      <div className="text-muted-foreground flex items-center justify-center gap-2 py-12 text-sm">
        <Loader2 className="size-4 animate-spin" /> Loading rule…
      </div>
    );
  }

  if (isEdit && ruleQuery.isError) {
    return (
      <Alert variant="destructive">
        <AlertTitle>Couldn't load this rule</AlertTitle>
        <AlertDescription>
          {ruleQuery.error instanceof Error
            ? ruleQuery.error.message
            : "Try refreshing the page."}
        </AlertDescription>
      </Alert>
    );
  }

  if (isEdit && !rule) {
    return (
      <Alert>
        <AlertTitle>Rule not found</AlertTitle>
        <AlertDescription>
          <Button asChild variant="link" className="px-0">
            <Link to="/rules">Back to rules</Link>
          </Button>
        </AlertDescription>
      </Alert>
    );
  }

  const title = isEdit ? `Edit rule` : "New rule";
  const description = isEdit
    ? "Changes apply on the next sync. Use 'Apply retroactively' on the detail page to re-run against existing transactions."
    : "When transactions match the conditions, the actions are applied automatically.";
  const submitting = isEdit ? updateRule.isPending : createRule.isPending;

  return (
    <>
      <PageHeader
        title={title}
        description={description}
        actions={
          <Button asChild variant="ghost" size="sm">
            <Link to="/rules">Back</Link>
          </Button>
        }
      />
      <RuleForm
        initialRule={rule}
        submitting={submitting}
        submitLabel={isEdit ? "Save changes" : "Create rule"}
        onSubmit={onSubmit}
        onCancel={goBack}
      />
    </>
  );
}
