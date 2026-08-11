import { useNavigate } from "react-router-dom";
import { ChevronDown, LogOut, Users } from "lucide-react";
import { useAuth } from "@/auth/auth";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const ROLE_LABEL: Record<string, string> = {
  SUPER_ADMIN: "Super admin",
  APP_ADMIN: "Admin de aplicación",
};

function initials(email?: string, role?: string): string {
  if (email) {
    const name = email.split("@")[0];
    const parts = name.split(/[._-]+/).filter(Boolean);
    const chars = parts.length >= 2 ? parts[0][0] + parts[1][0] : name.slice(0, 2);
    return chars.toUpperCase();
  }
  return (role ?? "U").slice(0, 1);
}

export default function UserMenu() {
  const { admin, logout } = useAuth();
  const navigate = useNavigate();
  if (!admin) return null;

  const superAdmin = admin.role === "SUPER_ADMIN";
  const roleLabel = ROLE_LABEL[admin.role] ?? admin.role;
  const fallback = initials(admin.email, admin.role);

  function signOut() {
    logout();
    navigate("/login");
  }

  return (
    <DropdownMenu>
      <DropdownMenuTrigger className="flex items-center gap-2 rounded-md py-1 pl-1 pr-2 text-sm text-foreground outline-none transition-colors hover:bg-accent focus-visible:ring-2 focus-visible:ring-ring/40 data-[state=open]:bg-accent">
        <Avatar>
          <AvatarFallback>{fallback}</AvatarFallback>
        </Avatar>
        <span className="hidden max-w-[12rem] truncate text-muted-foreground sm:block">{admin.email ?? roleLabel}</span>
        <ChevronDown className="size-4 text-muted-foreground transition-transform data-[state=open]:rotate-180" />
      </DropdownMenuTrigger>

      <DropdownMenuContent align="end" className="w-60">
        <div className="flex items-center gap-3 px-2 py-2">
          <Avatar className="h-9 w-9">
            <AvatarFallback className="text-sm">{fallback}</AvatarFallback>
          </Avatar>
          <div className="min-w-0">
            <p className="truncate text-sm font-medium text-foreground">{admin.email ?? "Administrador"}</p>
            <p className="truncate text-xs text-muted-foreground">{roleLabel}</p>
          </div>
        </div>

        <DropdownMenuSeparator />

        {superAdmin && (
          <DropdownMenuItem onClick={() => navigate("/admins")}>
            <Users />
            Usuarios admin
          </DropdownMenuItem>
        )}

        {superAdmin && <DropdownMenuSeparator />}

        <DropdownMenuItem variant="destructive" onClick={signOut}>
          <LogOut />
          Cerrar sesión
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
