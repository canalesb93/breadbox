import { Bell, Check, ChevronsUpDown, Plus } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Checkbox } from "@/components/ui/checkbox";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { Kbd, KbdGroup } from "@/components/ui/kbd";
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationLink,
  PaginationNext,
  PaginationPrevious,
} from "@/components/ui/pagination";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "@/components/ui/sheet";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import { SandboxSection, Specimen } from "@/sandbox/kit";

export function PrimitivesSection() {
  return (
    <SandboxSection
      title="Primitives"
      description="shadcn/ui components as themed for v2. Generated into components/ui/* — never hand-edited; variants come from theme tokens."
    >
      <Specimen label="Button" code="variant × size">
        <Button>Default</Button>
        <Button variant="secondary">Secondary</Button>
        <Button variant="outline">Outline</Button>
        <Button variant="ghost">Ghost</Button>
        <Button variant="destructive">Destructive</Button>
        <Button variant="link">Link</Button>
        <Button size="sm">Small</Button>
        <Button size="lg">Large</Button>
        <Button size="icon" aria-label="Add">
          <Plus />
        </Button>
        <Button disabled>Disabled</Button>
        <Button>
          <Bell /> With icon
        </Button>
      </Specimen>

      <Specimen
        label="Badge"
        code="variant"
        description="Pill-shaped by default. CategoryBadge overrides to rounded-md so pills stay reserved for tags."
      >
        <Badge>Default</Badge>
        <Badge variant="secondary">Secondary</Badge>
        <Badge variant="destructive">Destructive</Badge>
        <Badge variant="outline">Outline</Badge>
        <Badge variant="ghost">Ghost</Badge>
      </Specimen>

      <Specimen label="Input & Label" className="block">
        <div className="grid max-w-xs gap-1.5">
          <Label htmlFor="sb-input">Email</Label>
          <Input id="sb-input" placeholder="you@example.com" />
        </div>
        <div className="mt-3 grid max-w-xs gap-1.5">
          <Label htmlFor="sb-input-disabled">Disabled</Label>
          <Input id="sb-input-disabled" placeholder="Disabled" disabled />
        </div>
      </Specimen>

      <Specimen label="Textarea" className="block">
        <div className="grid max-w-md gap-1.5">
          <Label htmlFor="sb-textarea">Note</Label>
          <Textarea id="sb-textarea" placeholder="Add a note…" rows={3} />
        </div>
      </Specimen>

      <Specimen label="Checkbox">
        <Label className="flex items-center gap-2">
          <Checkbox defaultChecked /> Checked
        </Label>
        <Label className="flex items-center gap-2">
          <Checkbox /> Unchecked
        </Label>
        <Label className="flex items-center gap-2">
          <Checkbox checked="indeterminate" /> Indeterminate
        </Label>
        <Label className="text-muted-foreground flex items-center gap-2">
          <Checkbox disabled /> Disabled
        </Label>
      </Specimen>

      <Specimen label="RadioGroup" className="block">
        <RadioGroup defaultValue="full" className="gap-2">
          <Label className="flex items-center gap-2">
            <RadioGroupItem value="full" /> Full access
          </Label>
          <Label className="flex items-center gap-2">
            <RadioGroupItem value="read" /> Read only
          </Label>
          <Label className="text-muted-foreground flex items-center gap-2">
            <RadioGroupItem value="disabled" disabled /> Disabled
          </Label>
        </RadioGroup>
      </Specimen>

      <Specimen label="Card" className="block">
        <Card className="max-w-sm">
          <CardHeader>
            <CardTitle>Card title</CardTitle>
            <CardDescription>
              Header, content, and footer slots.
            </CardDescription>
          </CardHeader>
          <CardContent className="text-muted-foreground text-sm">
            Cards frame the detail page's Details and Activity panels.
          </CardContent>
          <CardFooter>
            <Button size="sm">Action</Button>
          </CardFooter>
        </Card>
      </Specimen>

      <Specimen label="Separator">
        <div className="text-sm">Above</div>
        <Separator className="my-2" />
        <div className="text-sm">Below</div>
        <div className="flex h-5 items-center gap-2 text-sm">
          Left <Separator orientation="vertical" /> Right
        </div>
      </Specimen>

      <Specimen
        label="Pagination"
        description="Numbered prev/next with ellipses — wrap with a feature-level helper that derives the page window from your data."
        className="block"
      >
        <Pagination>
          <PaginationContent>
            <PaginationItem>
              <PaginationPrevious href="#" />
            </PaginationItem>
            <PaginationItem>
              <PaginationLink href="#">1</PaginationLink>
            </PaginationItem>
            <PaginationItem>
              <PaginationLink href="#" isActive>
                2
              </PaginationLink>
            </PaginationItem>
            <PaginationItem>
              <PaginationLink href="#">3</PaginationLink>
            </PaginationItem>
            <PaginationItem>
              <PaginationEllipsis />
            </PaginationItem>
            <PaginationItem>
              <PaginationLink href="#">11</PaginationLink>
            </PaginationItem>
            <PaginationItem>
              <PaginationNext href="#" />
            </PaginationItem>
          </PaginationContent>
        </Pagination>
      </Specimen>

      <Specimen
        label="Skeleton"
        description="Loading placeholders — DataTable renders rows of these."
        className="block"
      >
        <div className="flex items-center gap-3">
          <Skeleton className="size-9 rounded-full" />
          <div className="space-y-2">
            <Skeleton className="h-4 w-40" />
            <Skeleton className="h-3 w-24" />
          </div>
        </div>
      </Specimen>

      <Specimen label="Kbd" code="Kbd · KbdGroup">
        <Kbd>⌘</Kbd>
        <Kbd>Esc</Kbd>
        <KbdGroup>
          <Kbd>⌘</Kbd>
          <Kbd>K</Kbd>
        </KbdGroup>
      </Specimen>

      <Specimen label="Avatar">
        <Avatar>
          <AvatarFallback>RC</AvatarFallback>
        </Avatar>
        <Avatar className="size-8">
          <AvatarFallback className="text-xs">v2</AvatarFallback>
        </Avatar>
      </Specimen>

      <Specimen
        label="Tooltip"
        description="Hover the button. KbdTooltip (see Patterns) wraps this with a key hint."
      >
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="outline">Hover me</Button>
          </TooltipTrigger>
          <TooltipContent>A tooltip</TooltipContent>
        </Tooltip>
      </Specimen>

      <Specimen
        label="Overlays"
        description="Popover, Dialog, Sheet, Dropdown — every floating surface in v2 is one of these. Click to open."
      >
        <Popover>
          <PopoverTrigger asChild>
            <Button variant="outline">Popover</Button>
          </PopoverTrigger>
          <PopoverContent className="text-sm">
            Anchored floating panel — used by the filter pills and pickers.
          </PopoverContent>
        </Popover>

        <Dialog>
          <DialogTrigger asChild>
            <Button variant="outline">Dialog</Button>
          </DialogTrigger>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>Dialog title</DialogTitle>
              <DialogDescription>
                Centered modal with a backdrop.
              </DialogDescription>
            </DialogHeader>
            <DialogFooter>
              <DialogClose asChild>
                <Button variant="outline">Close</Button>
              </DialogClose>
            </DialogFooter>
          </DialogContent>
        </Dialog>

        <Sheet>
          <SheetTrigger asChild>
            <Button variant="outline">Sheet</Button>
          </SheetTrigger>
          <SheetContent>
            <SheetHeader>
              <SheetTitle>Sheet title</SheetTitle>
              <SheetDescription>
                Edge-anchored panel — the shortcut reference uses this.
              </SheetDescription>
            </SheetHeader>
          </SheetContent>
        </Sheet>

        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="outline">
              Dropdown <ChevronsUpDown />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent>
            <DropdownMenuLabel>Menu</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem>First item</DropdownMenuItem>
            <DropdownMenuItem>Second item</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </Specimen>

      <Specimen
        label="Command"
        code="cmdk"
        description="The searchable list behind the command palette and every picker."
        className="block"
      >
        <Command className="max-w-sm rounded-lg border">
          <CommandInput placeholder="Search…" />
          <CommandList>
            <CommandEmpty>No results.</CommandEmpty>
            <CommandGroup heading="Suggestions">
              <CommandItem>
                <Check className="size-4" /> First result
              </CommandItem>
              <CommandItem>
                <Check className="size-4" /> Second result
              </CommandItem>
            </CommandGroup>
          </CommandList>
        </Command>
      </Specimen>

      <Specimen label="Breadcrumb">
        <Breadcrumb>
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink href="#">Money</BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage>Transactions</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>
      </Specimen>

      <Specimen label="Collapsible" className="block">
        <Collapsible className="max-w-sm space-y-2">
          <CollapsibleTrigger asChild>
            <Button variant="outline" size="sm">
              Toggle details <ChevronsUpDown />
            </Button>
          </CollapsibleTrigger>
          <CollapsibleContent className="text-muted-foreground text-sm">
            Collapsible content — the sidebar nav groups use this.
          </CollapsibleContent>
        </Collapsible>
      </Specimen>
    </SandboxSection>
  );
}
